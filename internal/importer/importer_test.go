package importer

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// groupCtx acts on the first group of a fresh database as ana, the first user, whose rules and history the import keeps.
var groupCtx = auth.WithUserID(scope.WithGroup(context.Background(), 1), 1)

// fixture holds a migrated database with a card that closes on the 3rd and is due on the 10th.
type fixture struct {
	s      *Service
	cards  *card.Service
	conn   *sql.DB
	userID int64
	cardID int64
	cat    map[string]int64
}

// dec is the invoice every test imports into.
var dec = civil.Month{Year: 2026, Month: time.December}

// newFixture opens a fresh database for one test.
func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := groupCtx
	conn := testutil.OpenDB(t)
	ana := testutil.NewUser(t, conn, "ana", "Ana Ficticia")
	testutil.FirstGroup(t, conn, ana)
	persons := []account.Person{{ID: ana.PersonID}}
	cards := card.NewService(conn).WithToday(func() civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: 20} })
	c, err := cards.CreateCard(ctx, ana.ID, card.CardInput{Name: "Nubank", PersonID: persons[0].ID, LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{s: NewService(conn, cards), cards: cards, conn: conn, userID: ana.ID, cardID: c.ID, cat: map[string]int64{}}
	for _, name := range []string{"Delivery", "Streaming", "Sem categoria definida", "Restaurantes e lanches", "Móveis e utilidades"} {
		var id int64
		if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatal(name, err)
		}
		f.cat[name] = id
	}
	return f
}

// invoiceCSV is a fictional December invoice with every kind of line.
const invoiceCSV = "date,title,amount\n" +
	"2026-11-05,IFOOD *PIZZARIA FICT,52.00\n" +
	"2026-11-06,Loja de Climatização - Parcela 3/10,359.99\n" +
	"2026-11-08,Mercado Ficticio,120.45\n" +
	"2026-11-09,Spotify,21.90\n" +
	"2026-11-12,Estorno Mercado Ficticio,-20.45\n" +
	"2026-11-15,Pagamento recebido,-900.00\n"

// TestRules picks the most recent rule for the same original text, ignoring case and spaces (RF-54, RN-19).
func TestRules(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	delivery, rest := f.cat["Delivery"], f.cat["Restaurantes e lanches"]
	tests := []struct {
		name    string
		in      RuleInput
		wantErr error
	}{
		{name: "first rule", in: RuleInput{Original: "ifood *pizzaria fict", FriendlyName: "iFood", CategoryID: &delivery}},
		{name: "newer rule for the same text", in: RuleInput{Original: "  IFOOD  *PIZZARIA FICT ", FriendlyName: "Pizzaria", CategoryID: &rest}},
		{name: "without category", in: RuleInput{Original: "Mercado Ficticio", FriendlyName: "Mercado do bairro"}},
		{name: "empty original", in: RuleInput{FriendlyName: "X"}, wantErr: ErrRuleOriginal},
		{name: "empty name", in: RuleInput{Original: "X"}, wantErr: ErrRuleName},
		{name: "top level category", in: RuleInput{Original: "X", FriendlyName: "X", CategoryID: ptr(int64(1))}, wantErr: ErrRuleCategory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.s.CreateRule(ctx, f.userID, tt.in); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	lines, err := f.s.Preview(ctx, f.cardID, dec, invoiceCSV)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].FriendlyName != "Pizzaria" || *lines[0].CategoryID != rest || lines[0].RuleID == nil {
		t.Fatalf("ifood line = %+v", lines[0])
	}
	if lines[2].FriendlyName != "Mercado do bairro" || *lines[2].CategoryID != f.cat["Sem categoria definida"] {
		t.Fatalf("market line = %+v", lines[2])
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// TestPreview applies rules, flags duplicates within a day and skips the payment (RF-53, RF-55, RF-56, RN-20).
func TestPreview(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	// Already in the Radar: the market purchase typed a day off, and the streaming recurrence.
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.November, Day: 7}, Merchant: "Mercado", CategoryID: f.cat["Delivery"], AmountCents: 12_045, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	// Two days off is not a duplicate.
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.November, Day: 3}, Merchant: "Pizza", CategoryID: f.cat["Delivery"], AmountCents: 5_200, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cards.AddRecurrence(ctx, f.userID, card.RecurrenceInput{CardID: f.cardID, Description: "Streaming", CategoryID: f.cat["Streaming"], AmountCents: 2_190, StartDate: civil.Date{Year: 2026, Month: time.November, Day: 4}}); err != nil {
		t.Fatal(err)
	}
	lines, err := f.s.Preview(ctx, f.cardID, dec, invoiceCSV)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind      LineKind
		duplicate bool
		skip      bool
	}{
		{kind: LinePurchase},
		{kind: LinePurchase},
		{kind: LinePurchase, duplicate: true, skip: true},
		{kind: LinePurchase, duplicate: true, skip: true},
		{kind: LineCredit},
		{kind: LinePayment, skip: true},
	}
	if len(lines) != len(want) {
		t.Fatalf("lines = %+v", lines)
	}
	for i, w := range want {
		l := lines[i]
		if l.Kind != w.kind || (l.Duplicate != nil) != w.duplicate || l.Skip != w.skip {
			t.Errorf("line %d = %+v, want %+v", i, l, w)
		}
	}
	if lines[2].Duplicate.Description != "Mercado" || lines[3].Duplicate.Description != "Streaming" {
		t.Fatalf("duplicates = %+v, %+v", lines[2].Duplicate, lines[3].Duplicate)
	}
	if lines[0].CategoryID == nil || *lines[0].CategoryID != f.cat["Sem categoria definida"] || lines[4].CategoryID != nil {
		t.Fatalf("default categories = %+v, %+v", lines[0].CategoryID, lines[4].CategoryID)
	}

	if _, err := f.s.Preview(ctx, 999, dec, invoiceCSV); !errors.Is(err, card.ErrCardNotFound) {
		t.Fatalf("missing card: %v", err)
	}
	if _, err := f.s.Preview(ctx, f.cardID, dec, "x,y\n"); !errors.Is(err, ErrNotNubank) {
		t.Fatalf("not a Nubank file: %v", err)
	}
}

// TestCommit records the chosen lines in the chosen invoice, learns the rules and can be undone (RF-53, RF-55, RF-57).
func TestCommit(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	lines, err := f.s.Preview(ctx, f.cardID, dec, invoiceCSV)
	if err != nil {
		t.Fatal(err)
	}
	commit := make([]CommitLine, 0, len(lines))
	for _, l := range lines {
		c := CommitLine{Line: l.Line, FriendlyName: l.FriendlyName, CategoryID: l.CategoryID, Skip: l.Skip}
		if l.Index == 0 {
			delivery := f.cat["Delivery"]
			c.FriendlyName, c.CategoryID, c.Remember = "iFood", &delivery, true
		}
		if l.Index == 1 {
			moveis := f.cat["Móveis e utilidades"]
			c.CategoryID = &moveis
		}
		commit = append(commit, c)
	}
	res, err := f.s.Commit(ctx, f.userID, f.cardID, dec, "Nubank_2026-12-10.csv", commit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 5 || res.Skipped != 1 || res.RulesSaved != 1 {
		t.Fatalf("result = %+v", res)
	}

	// Everything lands on the December invoice, whatever the date (RN-01): 52,00 + 359,99 + 120,45 + 21,90 - 20,45.
	inv, err := f.cards.Invoice(ctx, f.cardID, dec)
	if err != nil {
		t.Fatal(err)
	}
	if inv.TotalCents != 53_389 || len(inv.Items) != 5 {
		t.Fatalf("december = %d with %d items", inv.TotalCents, len(inv.Items))
	}
	// The installment 3 of 10 creates 3 to 10, the last one in July 2027, and nothing before December.
	var count int
	var last, firstMonth string
	_ = f.conn.QueryRow(`SELECT count(*), max(invoice_month), min(invoice_month) FROM installments i JOIN purchases p ON p.id = i.purchase_id WHERE p.merchant = 'Loja de Climatização'`).Scan(&count, &last, &firstMonth)
	if count != 8 || last != "2027-07" || firstMonth != "2026-12" {
		t.Fatalf("installments = %d from %s to %s", count, firstMonth, last)
	}

	// The rule learned on this import applies to the next one (RF-55).
	next, _ := f.s.Preview(ctx, f.cardID, dec.Add(1), invoiceCSV)
	if next[0].FriendlyName != "iFood" || *next[0].CategoryID != f.cat["Delivery"] {
		t.Fatalf("next import = %+v", next[0])
	}

	imports, _ := f.s.Imports(ctx)
	if len(imports) != 1 || imports[0].PurchaseCount != 5 || imports[0].CardName != "Nubank" {
		t.Fatalf("imports = %+v", imports)
	}
	if err := f.s.Undo(ctx, imports[0].ID); err != nil {
		t.Fatal(err)
	}
	if inv, _ := f.cards.Invoice(ctx, f.cardID, dec); inv.TotalCents != 0 {
		t.Fatalf("after undo = %d", inv.TotalCents)
	}
	if _, err := f.s.Commit(ctx, f.userID, f.cardID, dec, "x.csv", nil); !errors.Is(err, ErrNothingToImport) {
		t.Fatalf("empty commit: %v", err)
	}
}

// TestPreviewInstallments links an installment line to the purchase already in the Radar by number, count,
// amount and invoice, since the CSV dates later installments at the start of the cycle (RF-57, RN-20).
func TestPreviewInstallments(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	// R$ 400,00 in 4 bought on September 20: installments in October, November, December and January.
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.September, Day: 20}, Merchant: "Loja Parcelada", CategoryID: f.cat["Delivery"], AmountCents: 40_000, Installments: 4}); err != nil {
		t.Fatal(err)
	}
	// A one-off purchase with the same amount near the CSV date must not swallow an installment line.
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.November, Day: 4}, Merchant: "Avulsa", CategoryID: f.cat["Delivery"], AmountCents: 10_000, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	csv := "date,title,amount\n" +
		"2026-11-04,Loja Parcelada Ficticia - Parcela 3/4,\"100,00\"\n" +
		"2026-11-04,Outra Loja - Parcela 2/4,\"100,00\"\n" +
		"2026-11-04,Pix no Crédito - Fulano Ficticio - 3/4,\"99,99\"\n"
	lines, err := f.s.Preview(ctx, f.cardID, dec, csv)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		dup  string
	}{
		{name: "same number, count and amount on the invoice", dup: "Loja Parcelada"},
		{name: "number of another invoice"},
		{name: "other amount"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lines[i]
			got := ""
			if l.Duplicate != nil {
				got = l.Duplicate.Description
			}
			if got != tt.dup || l.Skip != (tt.dup != "") {
				t.Fatalf("line = %+v, duplicate %q", l, got)
			}
		})
	}
}

// TestPreviewMatchesOnce lets each purchase, installment or recurrence in the Radar flag only one CSV line,
// so a second identical charge is imported (RF-56, RN-20).
func TestPreviewMatchesOnce(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	nov := func(d int) civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: d} }
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: nov(10), Merchant: "Cafe", CategoryID: f.cat["Delivery"], AmountCents: 800, Installments: 1}); err != nil {
		t.Fatal(err)
	}
	// R$ 400,00 in 4 bought on September 20: installment 3 is on the December invoice.
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.September, Day: 20}, Merchant: "Loja Parcelada", CategoryID: f.cat["Delivery"], AmountCents: 40_000, Installments: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cards.AddRecurrence(ctx, f.userID, card.RecurrenceInput{CardID: f.cardID, Description: "Streaming", CategoryID: f.cat["Streaming"], AmountCents: 2_190, StartDate: nov(4)}); err != nil {
		t.Fatal(err)
	}
	csv := "date,title,amount\n" +
		"2026-11-10,Cafe Ficticio,\"8,00\"\n" +
		"2026-11-10,Cafe Ficticio,\"8,00\"\n" +
		"2026-11-04,Loja Parcelada - Parcela 3/4,\"100,00\"\n" +
		"2026-11-04,Outra Parcelada - Parcela 3/4,\"100,00\"\n" +
		"2026-11-09,Spotify,\"21,90\"\n" +
		"2026-11-19,Outro Streaming,\"21,90\"\n"
	lines, err := f.s.Preview(ctx, f.cardID, dec, csv)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, true, false, true, false}
	for i, w := range want {
		if (lines[i].Duplicate != nil) != w || lines[i].Skip != w {
			t.Errorf("line %d (%s) duplicate = %v, want %v", i, lines[i].Merchant, lines[i].Duplicate != nil, w)
		}
	}
}

// TestPreviewSingleLineSkipsInstallments keeps a one-shot CSV line from taking an installment of a purchase
// bought the same day, which left the real installment line to be imported twice (RF-57, RN-20).
func TestPreviewSingleLineSkipsInstallments(t *testing.T) {
	f := newFixture(t)
	ctx := groupCtx
	if _, err := f.cards.AddPurchase(ctx, f.userID, card.PurchaseInput{CardID: f.cardID, Date: civil.Date{Year: 2026, Month: time.November, Day: 10}, Merchant: "Loja", CategoryID: f.cat["Delivery"], AmountCents: 30_000, Installments: 3}); err != nil {
		t.Fatal(err)
	}
	csv := "date,title,amount\n" +
		"2026-11-10,Outra Loja,\"100,00\"\n" +
		"2026-11-10,Loja - Parcela 1/3,\"100,00\"\n"
	lines, err := f.s.Preview(ctx, f.cardID, dec, csv)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].Duplicate != nil || lines[1].Duplicate == nil {
		t.Fatalf("one-shot duplicate = %v, installment duplicate = %v", lines[0].Duplicate != nil, lines[1].Duplicate != nil)
	}
}

// TestCommitSplit divides imported lines with other people or leaves them all to one person; on an
// installment line the share follows only the installments created from the CSV on (RF-24, RF-25, RF-57).
func TestCommitSplit(t *testing.T) {
	pedroAll := &card.SplitInput{Mode: card.SplitEqual, CoupleParts: 0, Parties: []card.PartyShare{{Name: "Pedro Ficticio"}}}
	halfPedro := &card.SplitInput{Mode: card.SplitEqual, CoupleParts: 1, Parties: []card.PartyShare{{Name: "Pedro Ficticio"}}}
	tests := []struct {
		name      string
		index     int
		split     *card.SplitInput
		wantErr   error
		wantShare int64
		wantCount int
	}{
		{name: "everything to Pedro", index: 0, split: pedroAll, wantShare: 5_200, wantCount: 1},
		// 359,99 in two equal parts: Pedro 179,99 and the couple keeps the leftover cent, on installments 3 to 10.
		{name: "half of each installment", index: 1, split: halfPedro, wantShare: 17_999, wantCount: 8},
		{name: "custom value per installment", index: 2, split: &card.SplitInput{Mode: card.SplitCustom, Parties: []card.PartyShare{{Name: "Pedro Ficticio", ShareCents: 1_000}}}, wantShare: 1_000, wantCount: 1},
		{name: "share above the line", index: 2, split: &card.SplitInput{Mode: card.SplitCustom, Parties: []card.PartyShare{{Name: "Pedro Ficticio", ShareCents: 99_999}}}, wantErr: card.ErrSplitTooLarge},
		{name: "refund cannot be split", index: 4, split: pedroAll, wantErr: card.ErrSplitCredit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			ctx := groupCtx
			lines, err := f.s.Preview(ctx, f.cardID, dec, invoiceCSV)
			if err != nil {
				t.Fatal(err)
			}
			l := lines[tt.index]
			_, err = f.s.Commit(ctx, f.userID, f.cardID, dec, "x.csv", []CommitLine{{Line: l.Line, FriendlyName: l.FriendlyName, CategoryID: l.CategoryID, Split: tt.split}})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			var count int
			var each, total int64
			_ = f.conn.QueryRow(`SELECT count(*), max(si.amount_cents), sum(si.amount_cents) FROM split_installments si`).Scan(&count, &each, &total)
			if count != tt.wantCount || each != tt.wantShare || total != tt.wantShare*int64(tt.wantCount) {
				t.Fatalf("shares = %d of %d, total %d", count, each, total)
			}
			var stored int64
			_ = f.conn.QueryRow(`SELECT share_cents FROM splits`).Scan(&stored)
			if stored != total {
				t.Fatalf("split total = %d, want %d", stored, total)
			}
			inv, _ := f.cards.Invoice(ctx, f.cardID, dec)
			if inv.ThirdPartyCents != tt.wantShare {
				t.Fatalf("december third party = %d", inv.ThirdPartyCents)
			}
		})
	}
}

// TestGroupIsolation shows that rules and imports belong to the person, that purchases belong to the active
// group, and that nothing of another group is seen by duplicate detection (RF-54, RF-82, RN-20, RN-27).
func TestGroupIsolation(t *testing.T) {
	f := newFixture(t)
	delivery := f.cat["Delivery"]
	lines, err := f.s.Preview(groupCtx, f.cardID, dec, invoiceCSV)
	if err != nil {
		t.Fatal(err)
	}
	commit := make([]CommitLine, 0, len(lines))
	for _, l := range lines {
		commit = append(commit, CommitLine{Line: l.Line, FriendlyName: l.FriendlyName, CategoryID: l.CategoryID, Skip: l.Skip, Remember: l.Index == 0})
	}
	commit[0].CategoryID = &delivery
	res, err := f.s.Commit(groupCtx, f.userID, f.cardID, dec, "Nubank.csv", commit)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := f.s.Rules(groupCtx)
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules = %v, %v", rules, err)
	}

	// A second member of the same group has no rule and no import of Ana.
	bruno := testutil.NewUser(t, f.conn, "bruno", "Bruno Ficticio")
	accounts := account.NewService(f.conn)
	if err := accounts.AddMember(context.Background(), 1, bruno.ID, account.RoleMember); err != nil {
		t.Fatal(err)
	}
	asBruno := auth.WithUserID(groupCtx, bruno.ID)
	// A second group has its own card, bought with the same amounts, and its own member.
	carla := testutil.NewUser(t, f.conn, "carla", "Carla Ficticia")
	other := auth.WithUserID(testutil.NewGroup(t, f.conn, "Outra", carla), carla.ID)
	carlas, err := f.cards.CreateCard(other, carla.ID, card.CardInput{Name: "Nubank", PersonID: carla.PersonID, LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}

	for _, ctx := range []struct {
		name string
		ctx  context.Context
	}{{"another member", asBruno}, {"another group", other}} {
		t.Run(ctx.name, func(t *testing.T) {
			if got, err := f.s.Rules(ctx.ctx); err != nil || len(got) != 0 {
				t.Fatalf("rules = %v, %v", got, err)
			}
			if got, err := f.s.Imports(ctx.ctx); err != nil || len(got) != 0 {
				t.Fatalf("imports = %v, %v", got, err)
			}
			if _, err := f.s.UpdateRule(ctx.ctx, rules[0].ID, RuleInput{Original: "x", FriendlyName: "x"}); !errors.Is(err, ErrRuleNotFound) {
				t.Fatalf("update rule: err = %v", err)
			}
			if err := f.s.DeleteRule(ctx.ctx, rules[0].ID); !errors.Is(err, ErrRuleNotFound) {
				t.Fatalf("delete rule: err = %v", err)
			}
			if err := f.s.Undo(ctx.ctx, res.ImportID); !errors.Is(err, ErrImportNotFound) {
				t.Fatalf("undo: err = %v", err)
			}
		})
	}
	if _, err := f.s.Preview(other, f.cardID, dec, invoiceCSV); !errors.Is(err, card.ErrCardNotFound) {
		t.Fatalf("preview on the card of another group: err = %v", err)
	}
	if _, err := f.s.Commit(other, carla.ID, f.cardID, dec, "x.csv", commit); !errors.Is(err, card.ErrCardNotFound) {
		t.Fatalf("commit on the card of another group: err = %v", err)
	}
	// The same file on the card of the other group finds no duplicate: it only looks at its own group.
	own, err := f.s.Preview(other, carlas.ID, dec, invoiceCSV)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range own {
		if l.Duplicate != nil {
			t.Fatalf("duplicate found across groups: %+v", l)
		}
		if l.RuleID != nil {
			t.Fatalf("rule of Ana applied to Carla: %+v", l)
		}
	}
	if got, _ := f.s.Imports(groupCtx); len(got) != 1 {
		t.Fatalf("imports of Ana = %d, want 1", len(got))
	}
}
