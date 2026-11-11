package main

import (
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Receipt is what the proof of concept extracts from a consumer receipt page.
type Receipt struct {
	StoreName  string `json:"storeName"`
	StoreCNPJ  string `json:"storeCnpj"`
	AccessKey  string `json:"accessKey"`
	IssuedAt   string `json:"issuedAt"`
	TotalCents int64  `json:"totalCents"`
	Items      []Item `json:"items"`
}

// Item is one product line of the receipt; prices are integer cents (RN-21).
type Item struct {
	Description    string `json:"description"`
	Code           string `json:"code"`
	Quantity       string `json:"quantity"`
	Unit           string `json:"unit"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	TotalCents     int64  `json:"totalCents"`
}

// ErrNoItems is returned when the page does not have the expected item table.
var ErrNoItems = errors.New("item table not found: the page layout differs from the national portal layout")

var (
	cnpjPattern   = regexp.MustCompile(`\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}`)
	keyPattern    = regexp.MustCompile(`(\d{4}\s?){11}`)
	issuedPattern = regexp.MustCompile(`Emiss[ãa]o:?\s*(\d{2}/\d{2}/\d{4}\s+\d{2}:\d{2}(:\d{2})?)`)
	spaces        = regexp.MustCompile(`\s+`)
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
	}
	page := clean(text(doc))
	rec.StoreCNPJ = digits(cnpjPattern.FindString(page))
	if m := keyPattern.FindString(page); m != "" {
		rec.AccessKey = digits(m)
	}
	if m := issuedPattern.FindStringSubmatch(page); m != nil {
		rec.IssuedAt = m[1]
	}
	table := findFirst(doc, func(n *html.Node) bool { return hasID(n, "tabResult") })
	if table == nil {
		return rec, ErrNoItems
	}
	for _, row := range findAll(table, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "tr" }) {
		item, ok := parseRow(row)
		if ok {
			rec.Items = append(rec.Items, item)
		}
	}
	if len(rec.Items) == 0 {
		return rec, ErrNoItems
	}
	if n := findFirst(doc, func(n *html.Node) bool { return hasClass(n, "txtMax") }); n != nil {
		rec.TotalCents, _ = parseCents(text(n))
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
	unitPrice, _ := parseCents(afterColon(span("RvlUnit")))
	total, _ := parseCents(span("valor"))
	return Item{
		Description:    desc,
		Code:           strings.TrimSpace(strings.Trim(afterColon(span("RCod")), "()")),
		Quantity:       afterColon(span("Rqtd")),
		Unit:           afterColon(span("RUN")),
		UnitPriceCents: unitPrice,
		TotalCents:     total,
	}, true
}

// parseCents converts a Brazilian formatted amount like "1.234,56" to cents.
func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "R$", ""))
	s = strings.ReplaceAll(s, ".", "")
	whole, frac, _ := strings.Cut(s, ",")
	if len(frac) > 2 {
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	return strconv.ParseInt(whole+frac, 10, 64)
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

// clean collapses whitespace.
func clean(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
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
