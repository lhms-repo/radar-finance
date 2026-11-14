package importer

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/problem"
)

// Format names a CSV the Radar knows how to import (RF-53, RF-86).
type Format string

// Known formats: the invoice of a Nubank card and the current account statement of Santander.
const (
	FormatNubankInvoice      Format = "nubank_invoice"
	FormatSantanderStatement Format = "santander_statement"
)

// StatementKind tells what a line of an account statement becomes in the Radar (RF-87 to RF-89).
type StatementKind string

// Statement line kinds: money out, money in, the salary that may confirm an income and the payment of a card invoice.
const (
	StatementExpense     StatementKind = "expense"
	StatementIncome      StatementKind = "income"
	StatementSalary      StatementKind = "salary"
	StatementCardPayment StatementKind = "card_payment"
)

// System keys of the subcategories a statement expense falls in when no alias rule applies (RF-87).
const (
	KeyBankFees    = "bank_fees"
	KeyTaxes       = "taxes"
	KeyInterest    = "interest"
	KeyInvestments = "investments"
)

// Errors of reading a statement.
var (
	ErrNotSantander  = problem.Invalid("import_not_santander", "O arquivo não parece um extrato de conta do Santander. Exporte o extrato em CSV no app ou no site do Santander e tente de novo.")
	ErrUnknownFormat = problem.Invalid("import_unknown_format", "O Radar não reconhece este arquivo. Use o CSV da fatura do Nubank ou o CSV do extrato de conta do Santander.")
)

// StatementLine is one movement of the statement; AmountCents is always positive and Kind tells the direction.
type StatementLine struct {
	Date        civil.Date
	Description string
	Detail      string
	// Merchant is the detail without its leading date, or the description when there is no detail; the key of alias rules.
	Merchant    string
	AmountCents int64
	Kind        StatementKind
	// CategoryKey is the system key of the default subcategory of an expense, empty for Sem categoria definida.
	CategoryKey string
}

// Statement is a whole statement with its closing balance, the balance after the most recent line (RF-90).
type Statement struct {
	Lines        []StatementLine
	ClosingCents int64
}

var (
	// leadingDay is the "13/11 " the bank puts before the merchant of a debit card purchase.
	leadingDay = regexp.MustCompile(`^\d{2}/\d{2}\s+`)
	// brDate is the day of a statement line, DD/MM/YYYY.
	brDate = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4})$`)
)

// Detect tells which known CSV the content is, by its first lines (RF-86).
func Detect(content string) (Format, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	for i, line := range strings.SplitN(content, "\n", 20) {
		line = strings.TrimSpace(line)
		if i == 0 && strings.HasPrefix(strings.ToLower(line), "date,title,amount") {
			return FormatNubankInvoice, nil
		}
		if strings.EqualFold(line, "Extrato de conta corrente") || isStatementHeader(line) {
			return FormatSantanderStatement, nil
		}
	}
	return "", ErrUnknownFormat
}

// ParseSantander reads the current account statement exported by Santander: some lines about the account, then
// Data;Descrição;Detalhe;Valor;Saldo with dates as DD/MM/YYYY and amounts as -R$ 1.234,56, newest first.
func ParseSantander(r io.Reader) (Statement, error) {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && string(b) == "\ufeff" {
		_, _ = br.Discard(3)
	}
	n := 0
	for {
		line, err := br.ReadString('\n')
		if line == "" && errors.Is(err, io.EOF) {
			return Statement{}, ErrNotSantander
		}
		n++
		if isStatementHeader(strings.TrimSpace(line)) {
			break
		}
		if err != nil {
			return Statement{}, ErrNotSantander
		}
	}
	cr := csv.NewReader(br)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	var st Statement
	var balances []int64
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		n++
		if err != nil {
			return Statement{}, &LineError{Line: n, Reason: "formato inválido"}
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if len(rec) < 5 {
			return Statement{}, &LineError{Line: n, Reason: "esperava data, descrição, detalhe, valor e saldo"}
		}
		l, balance, err := parseStatementLine(rec)
		if err != nil {
			return Statement{}, &LineError{Line: n, Reason: err.Error()}
		}
		st.Lines = append(st.Lines, l)
		balances = append(balances, balance)
	}
	if len(st.Lines) == 0 {
		return Statement{}, ErrNothingToImport
	}
	// The newest line closes the period: the first one when the bank lists newest first, as it does, else the last.
	st.ClosingCents = balances[0]
	if st.Lines[0].Date.Before(st.Lines[len(st.Lines)-1].Date) {
		st.ClosingCents = balances[len(balances)-1]
	}
	return st, nil
}

// isStatementHeader tells the header of the statement lines, with or without accents.
func isStatementHeader(line string) bool {
	return fold(line) == "data;descricao;detalhe;valor;saldo"
}

// parseStatementLine reads one record of date, description, detail, amount and balance.
func parseStatementLine(rec []string) (StatementLine, int64, error) {
	m := brDate.FindStringSubmatch(strings.TrimSpace(rec[0]))
	if m == nil {
		return StatementLine{}, 0, errors.New("data inválida, esperava DD/MM/AAAA")
	}
	d, err := civil.ParseDate(m[3] + "-" + m[2] + "-" + m[1])
	if err != nil {
		return StatementLine{}, 0, errors.New("data inválida")
	}
	desc := spaces.ReplaceAllString(strings.TrimSpace(rec[1]), " ")
	detail := spaces.ReplaceAllString(strings.TrimSpace(rec[2]), " ")
	if desc == "" {
		return StatementLine{}, 0, errors.New("descrição vazia")
	}
	cents, err := parseCents(reais(rec[3]))
	if err != nil {
		return StatementLine{}, 0, err
	}
	if cents == 0 {
		return StatementLine{}, 0, errors.New("valor zerado")
	}
	balance, err := parseCents(reais(rec[4]))
	if err != nil {
		return StatementLine{}, 0, errors.New("saldo inválido")
	}
	merchant := leadingDay.ReplaceAllString(detail, "")
	if merchant == "" {
		merchant = desc
	}
	l := StatementLine{Date: d, Description: desc, Detail: detail, Merchant: merchant, AmountCents: cents, Kind: StatementExpense}
	key := fold(desc)
	switch {
	case cents > 0 && strings.Contains(key, "salario"):
		l.Kind = StatementSalary
	case cents > 0:
		l.Kind = StatementIncome
	case strings.HasPrefix(key, "pagamento cartao credito"):
		l.Kind = StatementCardPayment
	case strings.Contains(key, "tarifa"):
		l.CategoryKey = KeyBankFees
	case strings.Contains(key, "iof"):
		l.CategoryKey = KeyTaxes
	case strings.Contains(key, "juros"):
		l.CategoryKey = KeyInterest
	case strings.Contains(key, "aplicacao"):
		l.CategoryKey = KeyInvestments
	}
	if cents < 0 {
		l.AmountCents = -cents
	}
	return l, balance, nil
}

// reais drops the currency sign and the spaces of an amount like -R$ 1.234,56.
func reais(s string) string {
	s = strings.ReplaceAll(s, "R$", "")
	return strings.Join(strings.Fields(s), "")
}

// accents maps the accented letters of the bank texts to plain ones.
var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")

// fold lowers a text and drops its accents, to compare bank texts.
func fold(s string) string {
	return accents.Replace(strings.ToLower(strings.TrimSpace(s)))
}
