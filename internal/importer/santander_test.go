package importer

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestParseSantander reads the fictional statement: each line with its kind, the merchant used by the alias rules,
// the amount in cents and the default category, and the closing balance of the period (RF-86, RF-87).
func TestParseSantander(t *testing.T) {
	f, err := os.Open("testdata/santander-ficticio.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := ParseSantander(f)
	if err != nil {
		t.Fatal(err)
	}
	if st.ClosingCents != 187_650 || st.ClosingOn.String() != "2026-11-14" {
		t.Fatalf("closing balance = %d on %s, want 187650 on 2026-11-14", st.ClosingCents, st.ClosingOn)
	}
	want := []struct {
		date     string
		kind     StatementKind
		merchant string
		cents    int64
		key      string
	}{
		{"2026-11-14", StatementExpense, "PADARIA FICTICIA", 2_350, ""},
		{"2026-11-12", StatementExpense, "Fulano Ficticio", 10_000, ""},
		{"2026-11-10", StatementCardPayment, "Pagamento cartao credito bce", 45_000, ""},
		{"2026-11-05", StatementSalary, "EMPRESA FICTICIA LTDA", 250_000, ""},
		{"2026-11-04", StatementExpense, "Tarifa mensalidade pacote servicos", 3_290, KeyBankFees},
		{"2026-11-04", StatementExpense, "Iof imposto operacoes financeiras", 123, KeyTaxes},
		{"2026-11-04", StatementExpense, "Juros saldo utiliz ate limite", 587, KeyInterest},
		{"2026-11-03", StatementExpense, "Aplicacao cdb/rdb", 20_000, KeyInvestments},
		{"2026-11-03", StatementIncome, "Remuneracao aplicacao automatica", 15, ""},
		{"2026-11-02", StatementIncome, "Resgate cdb/rdb", 100_000, ""},
		{"2026-11-02", StatementIncome, "Beltrano Ficticio", 8_000, ""},
		{"2026-11-01", StatementIncome, "Mercado Ficticio", 985, ""},
	}
	if len(st.Lines) != len(want) {
		t.Fatalf("lines = %d, want %d", len(st.Lines), len(want))
	}
	for i, w := range want {
		l := st.Lines[i]
		if l.Date.String() != w.date || l.Kind != w.kind || l.Merchant != w.merchant || l.AmountCents != w.cents || l.CategoryKey != w.key {
			t.Errorf("line %d = %+v, want %+v", i, l, w)
		}
	}
}

// TestParseSantanderErrors refuses files that are not a Santander statement and points to the line it cannot read.
func TestParseSantanderErrors(t *testing.T) {
	header := "Extrato de conta corrente\nLançamentos:\nData;Descrição;Detalhe;Valor;Saldo\n"
	tests := []struct {
		name    string
		content string
		want    error
		line    int
	}{
		{name: "nubank invoice", content: "date,title,amount\n2026-11-01,Loja,10.00\n", want: ErrNotSantander},
		{name: "no header", content: "Extrato de conta corrente\n01/11/2026;Pix;;R$ 1,00;R$ 1,00\n", want: ErrNotSantander},
		{name: "bad date", content: header + "2026-11-01;Pix enviado;X;-R$ 1,00;R$ 1,00\n", line: 4},
		{name: "bad amount", content: header + "01/11/2026;Pix enviado;X;-R$ abc;R$ 1,00\n", line: 4},
		{name: "zero amount", content: header + "01/11/2026;Pix enviado;X;R$ 0,00;R$ 1,00\n", line: 4},
		{name: "missing columns", content: header + "01/11/2026;Pix enviado;X\n", line: 4},
		{name: "no lines", content: header, want: ErrNothingToImport},
		{name: "only a previous balance", content: header + "01/11/2026;SALDO ANTERIOR;;;R$ 1,00\n", want: ErrNothingToImport},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSantander(strings.NewReader(tt.content))
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("err = %v, want %v", err, tt.want)
				}
				return
			}
			var le *LineError
			if !errors.As(err, &le) || le.Line != tt.line {
				t.Fatalf("err = %v, want an error on line %d", err, tt.line)
			}
		})
	}
}

// TestDetect tells the Nubank invoice from the Santander statement by the content (RF-86).
func TestDetect(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Format
		err     error
	}{
		{name: "nubank", content: "date,title,amount\n2026-11-01,Loja,10.00\n", want: FormatNubankInvoice},
		{name: "nubank with BOM", content: "\ufeffdate,title,amount\n", want: FormatNubankInvoice},
		{name: "santander", content: "\ufeffExtrato de conta corrente\nCliente;X\n", want: FormatSantanderStatement},
		{name: "santander header only", content: "Data;Descrição;Detalhe;Valor;Saldo\n", want: FormatSantanderStatement},
		{name: "unknown", content: "foo;bar\n", err: ErrUnknownFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(tt.content)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("Detect = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

// TestParseSantanderTolerates reads variations of the export: quoted header with trailing separators, a previous
// balance row, a line without balance and a single day listed newest first (RF-86, RF-90).
func TestParseSantanderTolerates(t *testing.T) {
	content := "\ufeffExtrato de conta corrente;;;;\r\n" +
		"\"Data\";\"Descrição\";\"Detalhe\";\"Valor\";\"Saldo\";\r\n" +
		"02/11/2026;Pix enviado;Fulano;-R$ 1.234,56;R$ 765,44;\r\n" +
		"02/11/2026;Pix recebido;Beltrano;R$ 2.000,00;;\r\n" +
		"01/11/2026;SALDO ANTERIOR;;;R$ 0,00\r\n"
	st, err := ParseSantander(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Lines) != 2 || st.Lines[0].AmountCents != 123_456 || st.ClosingCents != 76_544 || st.ClosingOn.String() != "2026-11-02" {
		t.Fatalf("statement = %+v", st)
	}
}
