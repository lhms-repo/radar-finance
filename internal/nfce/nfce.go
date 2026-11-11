// Package nfce reads a consumer receipt (NFC-e) from the address of its QR code:
// it downloads the page from the state portal and extracts the store, the date
// and every item with quantity and unit price (RF-68).
package nfce

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/italogfx/radar-finance/internal/civil"
)

// Receipt is what a receipt page tells: store, access key, issue date, total and items.
type Receipt struct {
	StoreName    string
	StoreCNPJ    string
	Neighborhood string
	AccessKey    string
	IssuedOn     civil.Date
	TotalCents   int64
	Items        []Item
}

// Item is one product line; quantities are thousandths and prices are cents truncated from the receipt (RN-21).
type Item struct {
	Description    string
	Code           string
	QuantityMilli  int64
	Unit           string
	UnitPriceCents int64
	TotalCents     int64
}

// ErrNoItems is returned when the page does not have the expected item table.
var ErrNoItems = errors.New("item table not found: the page layout differs from the national portal layout")

var (
	cnpjPattern   = regexp.MustCompile(`\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}`)
	keyPattern    = regexp.MustCompile(`(\d{4}\s?){11}`)
	issuedPattern = regexp.MustCompile(`Emiss[ãa]o:?\s*(\d{2}/\d{2}/\d{4})`)
	spaces        = regexp.MustCompile(`\s+`)
	// amountPattern is a Brazilian amount with up to three decimals, as printed on receipts.
	amountPattern = regexp.MustCompile(`^(\d{1,3}(?:\.\d{3})*|\d+)(?:,(\d{1,3}))?$`)
)

// Parse reads the receipt page in the layout used by the state portals
// (table#tabResult with spans txtTit, RCod, Rqtd, RUN, RvlUnit and valor).
func Parse(r io.Reader) (Receipt, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Receipt{}, err
	}
	var rec Receipt
	if n := findFirst(doc, func(n *html.Node) bool { return hasID(n, "u20") || hasClass(n, "txtTopo") }); n != nil {
		rec.StoreName = clean(text(n))
		if addr := addressAfter(n); addr != "" {
			rec.Neighborhood = neighborhood(addr)
		}
	}
	page := clean(text(doc))
	rec.StoreCNPJ = digits(cnpjPattern.FindString(page))
	if m := keyPattern.FindString(page); m != "" {
		rec.AccessKey = digits(m)
	}
	if m := issuedPattern.FindStringSubmatch(page); m != nil {
		parts := strings.Split(m[1], "/")
		rec.IssuedOn, _ = civil.ParseDate(parts[2] + "-" + parts[1] + "-" + parts[0])
	}
	table := findFirst(doc, func(n *html.Node) bool { return hasID(n, "tabResult") })
	if table == nil {
		return rec, ErrNoItems
	}
	for _, row := range findAll(table, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "tr" }) {
		if item, ok := parseRow(row); ok {
			rec.Items = append(rec.Items, item)
		}
	}
	if len(rec.Items) == 0 {
		return rec, ErrNoItems
	}
	if n := findFirst(doc, func(n *html.Node) bool { return hasClass(n, "txtMax") }); n != nil {
		rec.TotalCents, _ = TruncateCents(text(n))
	}
	return rec, nil
}

// parseRow extracts one item from a table row, reporting false for other rows.
func parseRow(row *html.Node) (Item, bool) {
	span := func(class string) string {
		n := findFirst(row, func(n *html.Node) bool { return hasClass(n, class) })
		if n == nil {
			return ""
		}
		return clean(text(n))
	}
	desc := span("txtTit")
	if desc == "" {
		desc = span("txtTit2")
	}
	if desc == "" {
		return Item{}, false
	}
	unitPrice, _ := TruncateCents(afterColon(span("RvlUnit")))
	total, _ := TruncateCents(span("valor"))
	qty, err := quantityMilli(afterColon(span("Rqtd")))
	if err != nil {
		qty = 1_000
	}
	return Item{
		Description:    desc,
		Code:           strings.TrimSpace(strings.Trim(afterColon(span("RCod")), "()")),
		QuantityMilli:  qty,
		Unit:           afterColon(span("RUN")),
		UnitPriceCents: unitPrice,
		TotalCents:     total,
	}, true
}

// TruncateCents converts a receipt amount such as 6,499 to cents dropping the third decimal (RN-21).
func TruncateCents(s string) (int64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "R$", ""))
	m := amountPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	whole, err := strconv.ParseInt(strings.ReplaceAll(m[1], ".", ""), 10, 64)
	if err != nil {
		return 0, err
	}
	f, _ := strconv.ParseInt((m[2] + "00")[:2], 10, 64)
	return whole*100 + f, nil
}

// quantityMilli converts a receipt quantity such as 0,535 to thousandths.
func quantityMilli(s string) (int64, error) {
	m := amountPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid quantity %q", s)
	}
	whole, err := strconv.ParseInt(strings.ReplaceAll(m[1], ".", ""), 10, 64)
	if err != nil {
		return 0, err
	}
	f, _ := strconv.ParseInt((m[2] + "000")[:3], 10, 64)
	q := whole*1_000 + f
	if q <= 0 {
		return 0, fmt.Errorf("invalid quantity %q", s)
	}
	return q, nil
}

// addressAfter returns the text of the second element after the store name, the address line of the layout.
func addressAfter(store *html.Node) string {
	seen := 0
	for n := store.NextSibling; n != nil; n = n.NextSibling {
		if n.Type != html.ElementNode {
			continue
		}
		seen++
		if seen == 2 {
			return clean(text(n))
		}
	}
	return ""
}

// neighborhood picks the part before the city and the state of an address such as "RUA X, 10, , BAIRRO, CIDADE, UF".
func neighborhood(addr string) string {
	var parts []string
	for _, p := range strings.Split(addr, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-3]
}

// afterColon returns the text after the first colon, trimmed.
func afterColon(s string) string {
	if _, after, ok := strings.Cut(s, ":"); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(s)
}

// digits keeps only the digits of s.
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clean collapses whitespace, non-breaking spaces included.
func clean(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(strings.ReplaceAll(s, " ", " "), " "))
}

// text returns the concatenated text content of a node.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// findFirst returns the first node, in document order, that matches.
func findFirst(n *html.Node, match func(*html.Node) bool) *html.Node {
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, match); found != nil {
			return found
		}
	}
	return nil
}

// findAll returns every node that matches.
func findAll(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if match(n) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// hasID reports whether the element has the given id.
func hasID(n *html.Node, id string) bool {
	return attr(n, "id") == id
}

// hasClass reports whether the element has the given class.
func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// attr returns an attribute value of an element node.
func attr(n *html.Node, key string) string {
	if n.Type != html.ElementNode {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
