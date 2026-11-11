// Package importer reads the invoice CSV of Nubank, applies the alias rules,
// flags possible duplicates and records the purchases (RF-53 to RF-57, RN-19, RN-20).
package importer

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/problem"
)

// LineKind tells what a CSV line is.
type LineKind string

// Line kinds: purchases, refunds and credits, and the payment of the previous invoice, which is skipped.
const (
	LinePurchase LineKind = "purchase"
	LineCredit   LineKind = "credit"
	LinePayment  LineKind = "payment"
)

// ErrNotNubank is returned when the file does not start with the Nubank header.
var ErrNotNubank = problem.Invalid("import_not_nubank", "O arquivo não parece um CSV de fatura do Nubank. Exporte a fatura no app do Nubank e tente de novo.")

// LineError points to the line of the file that could not be read.
type LineError struct {
	Line   int
	Reason string
}

// Error returns the message in Portuguese, ready for the user.
func (e *LineError) Error() string { return fmt.Sprintf("Linha %d do arquivo: %s.", e.Line, e.Reason) }

// Line is one line of the invoice; AmountCents is always positive and Kind tells the direction.
type Line struct {
	Date  civil.Date
	Title string
	// Merchant is the title without the installment suffix and with single spaces, the key of alias rules (RN-19).
	Merchant     string
	AmountCents  int64
	Kind         LineKind
	Installment  int
	Installments int
}

var (
	installmentPattern = regexp.MustCompile(`(?i)^(.*?)\s*-\s*parcela\s+(\d+)\s*/\s*(\d+)\s*$`)
	spaces             = regexp.MustCompile(`\s+`)
	amountPattern      = regexp.MustCompile(`^(-?)(\d+)(?:\.(\d{1,2}))?$`)
)

// ParseNubank reads the CSV exported by Nubank for an invoice: header date,title,amount, dates as
// YYYY-MM-DD and amounts with a dot, positive for purchases and negative for credits and payments.
func ParseNubank(r io.Reader) ([]Line, error) {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && string(b) == "\ufeff" {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil || len(header) < 3 || !strings.EqualFold(strings.TrimSpace(header[0]), "date") ||
		!strings.EqualFold(strings.TrimSpace(header[1]), "title") || !strings.EqualFold(strings.TrimSpace(header[2]), "amount") {
		return nil, ErrNotNubank
	}
	var out []Line
	for n := 2; ; n++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &LineError{Line: n, Reason: "formato inválido"}
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if len(rec) != 3 {
			return nil, &LineError{Line: n, Reason: "esperava data, descrição e valor"}
		}
		line, err := parseLine(rec)
		if err != nil {
			return nil, &LineError{Line: n, Reason: err.Error()}
		}
		out = append(out, line)
	}
	return out, nil
}

// parseLine reads one record of date, title and amount.
func parseLine(rec []string) (Line, error) {
	d, err := civil.ParseDate(strings.TrimSpace(rec[0]))
	if err != nil {
		return Line{}, errors.New("data inválida, esperava AAAA-MM-DD")
	}
	title := strings.TrimSpace(rec[1])
	if title == "" {
		return Line{}, errors.New("descrição vazia")
	}
	cents, err := parseCents(strings.TrimSpace(rec[2]))
	if err != nil {
		return Line{}, err
	}
	if cents == 0 {
		return Line{}, errors.New("valor zerado")
	}
	l := Line{Date: d, Title: title, Merchant: spaces.ReplaceAllString(title, " "), AmountCents: cents, Kind: LinePurchase}
	if m := installmentPattern.FindStringSubmatch(title); m != nil {
		n, _ := strconv.Atoi(m[2])
		total, _ := strconv.Atoi(m[3])
		if n < 1 || total < 1 || n > total || total > 48 {
			return Line{}, errors.New("parcela inválida")
		}
		l.Merchant, l.Installment, l.Installments = spaces.ReplaceAllString(m[1], " "), n, total
	}
	if cents < 0 {
		l.AmountCents = -cents
		l.Kind = LineCredit
		if strings.HasPrefix(strings.ToLower(l.Merchant), "pagamento") {
			l.Kind = LinePayment
		}
	}
	return l, nil
}

// parseCents converts an amount with a dot and up to two decimals to cents, without floating point (RN-21).
func parseCents(s string) (int64, error) {
	m := amountPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, errors.New("valor inválido, esperava algo como 45.90")
	}
	whole, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || whole > 1_000_000_000 {
		return 0, errors.New("valor inválido")
	}
	frac := m[3]
	for len(frac) < 2 {
		frac += "0"
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	cents := whole*100 + f
	if m[1] == "-" {
		cents = -cents
	}
	return cents, nil
}
