package importer

import (
	"errors"
	"strings"
	"testing"
)

// TestParseNubank reads the invoice CSV of Nubank: purchases, installments, refunds and the payment (RF-53, RF-57).
func TestParseNubank(t *testing.T) {
	csv := "\ufeffdate,title,amount\r\n" +
		"2026-10-29,Mercado Ficticio,45.90\r\n" +
		"2026-10-30,Loja de Climatização - Parcela 3/10,359.99\r\n" +
		"2026-11-01,PIZZARIA   FICTICIA,52\r\n" +
		"2026-11-02,Estorno de Pizzaria Ficticia,-12.5\r\n" +
		"2026-11-03,Pagamento recebido,-1234.56\r\n" +
		"2026-11-04,\"Padaria, Doces e Cia\",8.70\r\n" +
		"\r\n" +
		// Current export: Brazilian amounts in quotes, a space after the minus and installments without the word.
		"2026-11-05,Padaria Ficticia,\"156,03\"\r\n" +
		"2026-11-06,Pagamento recebido,\"- 6.324,93\"\r\n" +
		"2026-10-30,Pix no Crédito - Fulano Ficticio - 2/2,\"384,22\"\r\n" +
		"2026-10-30,Loja Ficticia - NuPay - Parcela 4/10,\"1.116,91\"\r\n" +
		"2026-11-07,Estorno Ficticio,\"-20,45\"\r\n" +
		// A title ending in a month, not an installment: without the word, numbers that cannot be one stay in the title.
		"2026-11-08,Assinatura Ficticia - 11/2026,\"29,90\"\r\n"
	got, err := ParseNubank(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		date, title, merchant string
		cents                 int64
		kind                  LineKind
		n, m                  int
	}{
		{date: "2026-10-29", title: "Mercado Ficticio", merchant: "Mercado Ficticio", cents: 4_590, kind: LinePurchase},
		{date: "2026-10-30", title: "Loja de Climatização - Parcela 3/10", merchant: "Loja de Climatização", cents: 35_999, kind: LinePurchase, n: 3, m: 10},
		{date: "2026-11-01", title: "PIZZARIA   FICTICIA", merchant: "PIZZARIA FICTICIA", cents: 5_200, kind: LinePurchase},
		{date: "2026-11-02", title: "Estorno de Pizzaria Ficticia", merchant: "Estorno de Pizzaria Ficticia", cents: 1_250, kind: LineCredit},
		{date: "2026-11-03", title: "Pagamento recebido", merchant: "Pagamento recebido", cents: 123_456, kind: LinePayment},
		{date: "2026-11-04", title: "Padaria, Doces e Cia", merchant: "Padaria, Doces e Cia", cents: 870, kind: LinePurchase},
		{date: "2026-11-05", title: "Padaria Ficticia", merchant: "Padaria Ficticia", cents: 15_603, kind: LinePurchase},
		{date: "2026-11-06", title: "Pagamento recebido", merchant: "Pagamento recebido", cents: 632_493, kind: LinePayment},
		{date: "2026-10-30", title: "Pix no Crédito - Fulano Ficticio - 2/2", merchant: "Pix no Crédito - Fulano Ficticio", cents: 38_422, kind: LinePurchase, n: 2, m: 2},
		{date: "2026-10-30", title: "Loja Ficticia - NuPay - Parcela 4/10", merchant: "Loja Ficticia - NuPay", cents: 111_691, kind: LinePurchase, n: 4, m: 10},
		{date: "2026-11-07", title: "Estorno Ficticio", merchant: "Estorno Ficticio", cents: 2_045, kind: LineCredit},
		{date: "2026-11-08", title: "Assinatura Ficticia - 11/2026", merchant: "Assinatura Ficticia - 11/2026", cents: 2_990, kind: LinePurchase},
	}
	if len(got) != len(want) {
		t.Fatalf("lines = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		l := got[i]
		if l.Date.String() != w.date || l.Title != w.title || l.Merchant != w.merchant || l.AmountCents != w.cents || l.Kind != w.kind || l.Installment != w.n || l.Installments != w.m {
			t.Errorf("line %d = %+v, want %+v", i, l, w)
		}
	}
}

// TestParseNubankErrors rejects files that are not a Nubank invoice CSV.
func TestParseNubankErrors(t *testing.T) {
	tests := []struct {
		name string
		csv  string
		line int
	}{
		{name: "empty file", csv: ""},
		{name: "other header", csv: "data,descricao,valor\n2026-11-01,X,1.00\n"},
		{name: "bad date", csv: "date,title,amount\n01/11/2026,X,1.00\n", line: 2},
		{name: "bad amount", csv: "date,title,amount\n2026-11-01,X,1,00\n", line: 2},
		{name: "bad Brazilian amount", csv: "date,title,amount\n2026-11-01,X,\"1,234\"\n", line: 2},
		{name: "three decimals", csv: "date,title,amount\n2026-11-01,X,1.005\n", line: 2},
		{name: "zero amount", csv: "date,title,amount\n2026-11-01,X,0\n", line: 2},
		{name: "empty title", csv: "date,title,amount\n2026-11-01, ,1.00\n", line: 2},
		{name: "installment past the count", csv: "date,title,amount\n2026-11-01,X - Parcela 4/3,1.00\n", line: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseNubank(strings.NewReader(tt.csv))
			var le *LineError
			switch {
			case err == nil:
				t.Fatal("expected an error")
			case tt.line == 0 && !errors.Is(err, ErrNotNubank):
				t.Fatalf("err = %v, want ErrNotNubank", err)
			case tt.line != 0 && (!errors.As(err, &le) || le.Line != tt.line):
				t.Fatalf("err = %v, want a LineError on line %d", err, tt.line)
			}
		})
	}
}

// TestParseCents converts the amounts of the CSV, with a dot or in the Brazilian format, to cents without floating point (RN-21).
func TestParseCents(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"45.90", 4_590, true}, {"52", 5_200, true}, {"-12.5", -1_250, true}, {"0.01", 1, true}, {"1234.56", 123_456, true},
		{"1,00", 100, true}, {"156,03", 15_603, true}, {"6.324,93", 632_493, true}, {"- 6.324,93", -632_493, true}, {"-20,4", -2_040, true},
		{"1.234.567,89", 123_456_789, true}, {"12.34,56", 0, false}, {"1,234", 0, false}, {"1.234", 0, false},
		{"1.005", 0, false}, {"", 0, false}, {"abc", 0, false}, {"1.2.3", 0, false}, {"--1,00", 0, false},
	}
	for _, tt := range tests {
		got, err := parseCents(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("parseCents(%q) = %d, %v", tt.in, got, err)
		}
	}
}
