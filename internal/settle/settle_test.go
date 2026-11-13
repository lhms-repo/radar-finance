package settle

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/scope"
	"github.com/italogfx/radar-finance/internal/testutil"
)

// home is the group Casa of the fixture, created first.
var home = scope.WithGroup(context.Background(), 1)

// nov returns a day of November 2026.
func nov(d int) civil.Date { return civil.Date{Year: 2026, Month: time.November, Day: d} }

// trio holds a group of three members, Ana, Bruno and Carla, with an account each, and the services.
type trio struct {
	conn     *sql.DB
	s        *Service
	ledger   *ledger.Service
	cards    *card.Service
	people   map[string]account.User
	accounts map[string]int64
	cat      func(string) int64
}

// newTrio builds Casa with Ana, Bruno and Carla, weights of 50%, 30% and 20% and the settlement from October 1st,
// with today on November 15, 2026.
func newTrio(t *testing.T) trio {
	t.Helper()
	conn := testutil.OpenDB(t)
	people := map[string]account.User{
		"ana":   testutil.NewUser(t, conn, "ana", "Ana Ficticia"),
		"bruno": testutil.NewUser(t, conn, "bruno", "Bruno Ficticio"),
		"carla": testutil.NewUser(t, conn, "carla", "Carla Ficticia"),
	}
	testutil.FirstGroup(t, conn, people["ana"], people["bruno"], people["carla"])
	// The weights and the start of the settlement are fixed in the past, apart from the real clock.
	for _, q := range []string{
		`DELETE FROM group_weights`,
		`INSERT INTO group_weights (group_id, person_id, weight_bp, from_day) VALUES (1, 1, 5000, '2026-10-01'), (1, 2, 3000, '2026-10-01'), (1, 3, 2000, '2026-10-01')`,
		`UPDATE groups SET settles_from = '2026-10-01' WHERE id = 1`,
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	today := func() civil.Date { return nov(15) }
	cards := card.NewService(conn).WithToday(today)
	tr := trio{conn: conn, ledger: ledger.NewService(conn), cards: cards, people: people, accounts: map[string]int64{}}
	tr.s = NewService(conn, cards).WithToday(today)
	tr.cat = func(name string) int64 {
		var id int64
		if err := conn.QueryRow(`SELECT id FROM categories WHERE name = ?`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for _, login := range []string{"ana", "bruno", "carla"} {
		a, err := tr.ledger.CreateAccount(home, people[login].ID, ledger.NewAccount{Name: "Conta " + login, PersonID: people[login].PersonID, InitialBalanceCents: 1_000_000})
		if err != nil {
			t.Fatal(err)
		}
		tr.accounts[login] = a.ID
	}
	return tr
}

// rule saves a division between members for an item of Casa.
func (tr trio) rule(t *testing.T, kind membersplit.Kind, id int64, r *membersplit.Rule, amount int64) {
	t.Helper()
	if err := membersplit.Save(context.Background(), store.New(tr.conn), 1, kind, id, r, amount, time.Now(), true); err != nil {
		t.Fatal(err)
	}
}

// person returns the person id of a member.
func (tr trio) person(login string) int64 { return tr.people[login].PersonID }

// TestThreeMembersCloseToTheCent is the criterion of Fase 10: three members with weights, every kind of expense and
// division, balances that add up to zero to the cent, and nothing left after the suggested payments (RF-84, RN-28 to RN-30).
func TestThreeMembersCloseToTheCent(t *testing.T) {
	tr := newTrio(t)
	ctx := home
	// Ana pays R$ 100,00 from her account, by the weights: 50,00, 30,00 and 20,00.
	if _, err := tr.ledger.AddExpense(ctx, tr.people["ana"].ID, ledger.NewExpense{AccountID: tr.accounts["ana"], AmountCents: 10_000, Date: nov(2), CategoryID: tr.cat("Delivery")}); err != nil {
		t.Fatal(err)
	}
	// Bruno pays the power bill of R$ 90,01 in equal parts: the cent left goes to Ana, who joined first.
	bills := bill.NewService(tr.conn)
	power, err := bills.Create(ctx, tr.people["bruno"].ID, bill.Input{Name: "Energia", CategoryID: tr.cat("Energia"), AccountID: tr.accounts["bruno"], DueDay: 5, ExpectedCents: 9_001, StartMonth: civil.Month{Year: 2026, Month: time.November}})
	if err != nil {
		t.Fatal(err)
	}
	tr.rule(t, membersplit.KindBill, power.ID, &membersplit.Rule{Mode: membersplit.ModeEqual, Members: []membersplit.Member{{PersonID: tr.person("ana")}, {PersonID: tr.person("bruno")}, {PersonID: tr.person("carla")}}}, 0)
	if _, err := bills.Pay(ctx, tr.people["bruno"].ID, bill.Payment{BillID: power.ID, Month: civil.Month{Year: 2026, Month: time.November}, AmountCents: 9_001, Date: nov(5)}); err != nil {
		t.Fatal(err)
	}
	// Carla's card closes on the 3rd: R$ 300,00 in 3x from October 5 puts only the November installment in.
	c, err := tr.cards.CreateCard(ctx, tr.people["carla"].ID, card.CardInput{Name: "Cartao Carla", PersonID: tr.person("carla"), LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.cards.AddPurchase(ctx, tr.people["carla"].ID, card.PurchaseInput{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.October, Day: 5}, Merchant: "Sofa", CategoryID: tr.cat("Delivery"), AmountCents: 30_000, Installments: 3}); err != nil {
		t.Fatal(err)
	}
	// R$ 200,00 with R$ 50,00 of Pedro, from outside: the group divides R$ 150,00, Bruno 100,00 and Carla 50,00.
	shared, err := tr.cards.AddPurchase(ctx, tr.people["carla"].ID, card.PurchaseInput{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.October, Day: 10}, Merchant: "Jantar", CategoryID: tr.cat("Delivery"), AmountCents: 20_000, Installments: 1,
		Split: &card.SplitInput{Mode: card.SplitCustom, Parties: []card.PartyShare{{Name: "Pedro Ficticio", ShareCents: 5_000}}}})
	if err != nil {
		t.Fatal(err)
	}
	tr.rule(t, membersplit.KindPurchase, shared.ID, &membersplit.Rule{Mode: membersplit.ModeCustom, Members: []membersplit.Member{{PersonID: tr.person("bruno"), ShareCents: 10_000}, {PersonID: tr.person("carla"), ShareCents: 5_000}}}, 15_000)
	// Ana's Alelo pays R$ 33,33 split between her and Bruno: 16,67 and 16,66.
	alelos := alelo.NewService(tr.conn)
	ac, err := alelos.Create(ctx, tr.people["ana"].ID, alelo.Input{Name: "Alelo Ana", PersonID: tr.person("ana"), MonthlyCreditCents: 80_000, CreditDay: 1, Style: "tudo"})
	if err != nil {
		t.Fatal(err)
	}
	ap, err := alelos.AddPurchase(ctx, tr.people["ana"].ID, alelo.Purchase{AleloID: ac.ID, Date: nov(6), Description: "Feira", AmountCents: 3_333})
	if err != nil {
		t.Fatal(err)
	}
	tr.rule(t, membersplit.KindAlelo, ap.ID, &membersplit.Rule{Mode: membersplit.ModeEqual, Members: []membersplit.Member{{PersonID: tr.person("ana")}, {PersonID: tr.person("bruno")}}}, 0)
	// Before the start, and in Ana's personal space (id 2), nothing counts (RN-31, RF-83).
	if _, err := tr.ledger.AddExpense(ctx, tr.people["bruno"].ID, ledger.NewExpense{AccountID: tr.accounts["bruno"], AmountCents: 50_000, Date: civil.Date{Year: 2026, Month: time.September, Day: 20}, CategoryID: tr.cat("Delivery")}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.ledger.AddExpense(scope.WithGroup(context.Background(), 2), tr.people["ana"].ID, ledger.NewExpense{AccountID: tr.accounts["ana"], AmountCents: 7_777, Date: nov(7), CategoryID: tr.cat("Delivery")}); err != nil {
		t.Fatal(err)
	}

	st, err := tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.November})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]MemberBalance{
		tr.person("ana"):   {PaidCents: 13_333, ShareCents: 14_668, BalanceCents: -1_335},
		tr.person("bruno"): {PaidCents: 9_001, ShareCents: 20_666, BalanceCents: -11_665},
		tr.person("carla"): {PaidCents: 25_000, ShareCents: 12_000, BalanceCents: 13_000},
	}
	var sum int64
	for _, m := range st.Members {
		w := want[m.PersonID]
		if m.PaidCents != w.PaidCents || m.ShareCents != w.ShareCents || m.BalanceCents != w.BalanceCents {
			t.Errorf("%s = paid %d, share %d, balance %d; want %d, %d, %d", m.Name, m.PaidCents, m.ShareCents, m.BalanceCents, w.PaidCents, w.ShareCents, w.BalanceCents)
		}
		sum += m.BalanceCents
	}
	if sum != 0 || len(st.Members) != 3 {
		t.Fatalf("balances add up to %d over %d members, want zero over three (RN-29)", sum, len(st.Members))
	}
	if len(st.Lines) != 5 {
		t.Fatalf("lines of November = %d, want 5: %+v", len(st.Lines), st.Lines)
	}
	// Bruno pays Carla 116,65 and Ana pays her 13,35.
	if len(st.Suggestions) != 2 || st.Suggestions[0].FromPersonID != tr.person("bruno") || st.Suggestions[0].AmountCents != 11_665 ||
		st.Suggestions[1].FromPersonID != tr.person("ana") || st.Suggestions[1].AmountCents != 1_335 || st.Suggestions[1].ToPersonID != tr.person("carla") {
		t.Fatalf("suggestions = %+v", st.Suggestions)
	}
	from := map[int64]string{tr.person("ana"): "ana", tr.person("bruno"): "bruno"}
	for _, sg := range st.Suggestions {
		if _, err := tr.s.Pay(ctx, tr.people[from[sg.FromPersonID]].ID, PaymentInput{FromAccountID: tr.accounts[from[sg.FromPersonID]], ToAccountID: tr.accounts["carla"], AmountCents: sg.AmountCents, Date: nov(15)}); err != nil {
			t.Fatal(err)
		}
	}
	after, err := tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.November})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range after.Members {
		if m.BalanceCents != 0 {
			t.Errorf("%s after the payments = %d, want zero", m.Name, m.BalanceCents)
		}
	}
	if len(after.Suggestions) != 0 || len(after.Payments) != 2 {
		t.Fatalf("after the payments: suggestions %+v, payments %d", after.Suggestions, len(after.Payments))
	}
	// A payment between members is neither spending nor income (RN-30).
	if flow, _ := tr.ledger.MonthFlow(ctx, civil.Month{Year: 2026, Month: time.November}); flow.IncomeCents != 0 || flow.ExpenseCents != 10_000+9_001 {
		t.Fatalf("month flow = %+v", flow)
	}
	// Undoing a payment brings its part of the balance back.
	if err := tr.s.DeletePayment(ctx, tr.people["ana"].ID, after.Payments[0].ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.November}); len(again.Payments) != 1 || len(again.Suggestions) != 1 {
		t.Fatalf("after undoing a payment = %+v", again)
	}
}

// TestWeightsInForce uses the weights of the day of each expense and leaves out a member before she joined (RN-28).
func TestWeightsInForce(t *testing.T) {
	tr := newTrio(t)
	ctx := home
	// From November 10 the weights become equal thirds.
	if _, err := tr.conn.Exec(`INSERT INTO group_weights (group_id, person_id, weight_bp, from_day) VALUES (1, 1, 3334, '2026-11-10'), (1, 2, 3333, '2026-11-10'), (1, 3, 3333, '2026-11-10')`); err != nil {
		t.Fatal(err)
	}
	for _, d := range []int{9, 10} {
		if _, err := tr.ledger.AddExpense(ctx, tr.people["ana"].ID, ledger.NewExpense{AccountID: tr.accounts["ana"], AmountCents: 10_000, Date: nov(d), CategoryID: tr.cat("Delivery")}); err != nil {
			t.Fatal(err)
		}
	}
	st, err := tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.November})
	if err != nil {
		t.Fatal(err)
	}
	shares := map[int64]int64{}
	for _, m := range st.Members {
		shares[m.PersonID] = m.ShareCents
	}
	// November 9: 50, 30, 20; November 10: 33,34, 33,33, 33,33.
	if shares[tr.person("ana")] != 5_000+3_334 || shares[tr.person("bruno")] != 3_000+3_333 || shares[tr.person("carla")] != 2_000+3_333 {
		t.Fatalf("shares = %v", shares)
	}

	// Someone who joins later takes no part in what came before her.
	dani := testutil.NewUser(t, tr.conn, "dani", "Dani Ficticia")
	if err := account.NewService(tr.conn).AddMember(context.Background(), 1, dani.ID, account.RoleMember); err != nil {
		t.Fatal(err)
	}
	st, _ = tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.November})
	for _, m := range st.Members {
		if m.PersonID == dani.PersonID && (m.ShareCents != 0 || m.BalanceCents != 0) {
			t.Fatalf("new member took part in the past: %+v", m)
		}
	}
}

// TestPayRules refuses a payment between accounts of the same person and any settlement in a personal space (RN-30).
func TestPayRules(t *testing.T) {
	tr := newTrio(t)
	other, err := tr.ledger.CreateAccount(home, tr.people["ana"].ID, ledger.NewAccount{Name: "Outra Ana", PersonID: tr.person("ana")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.s.Pay(home, tr.people["ana"].ID, PaymentInput{FromAccountID: tr.accounts["ana"], ToAccountID: other.ID, AmountCents: 100, Date: nov(15)}); !errors.Is(err, ErrSamePerson) {
		t.Fatalf("payment to herself: %v, want ErrSamePerson", err)
	}
	if _, err := tr.s.Settlement(scope.WithGroup(context.Background(), 2), civil.Month{Year: 2026, Month: time.November}); !errors.Is(err, ErrPersonal) {
		t.Fatalf("settlement of a personal space: %v, want ErrPersonal", err)
	}
	tests := []struct {
		name    string
		by      string
		from    string
		to      string
		date    civil.Date
		wantErr error
	}{
		{name: "a member outside the payment", by: "carla", from: "ana", to: "bruno", date: nov(15), wantErr: ErrNotParty},
		{name: "a date in the future", by: "ana", from: "ana", to: "bruno", date: nov(16), wantErr: ErrPaymentDate},
		{name: "a date before the start", by: "ana", from: "ana", to: "bruno", date: civil.Date{Year: 2026, Month: time.September, Day: 30}, wantErr: ErrPaymentDate},
		{name: "who pays", by: "bruno", from: "bruno", to: "carla", date: nov(15)},
		{name: "who receives", by: "carla", from: "bruno", to: "carla", date: nov(15)},
		{name: "an administrator outside the payment", by: "ana", from: "bruno", to: "carla", date: nov(15)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.s.Pay(home, tr.people[tt.by].ID, PaymentInput{FromAccountID: tr.accounts[tt.from], ToAccountID: tr.accounts[tt.to], AmountCents: 100, Date: tt.date})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
	st, _ := tr.s.Settlement(home, civil.Month{Year: 2026, Month: time.November})
	if err := tr.s.DeletePayment(home, tr.people["carla"].ID, st.Payments[0].ID); err != nil {
		t.Fatalf("undo by who received: %v", err)
	}
	if err := tr.s.DeletePayment(home, tr.people["ana"].ID, st.Payments[1].ID); err != nil {
		t.Fatalf("undo by an administrator: %v", err)
	}
}

// TestChangesApplyFromNowOn keeps what was paid or recorded before a change with the division and the weights it had:
// a bill paid before its division changes, an expense recorded the same day before someone joins (RN-28).
func TestChangesApplyFromNowOn(t *testing.T) {
	tr := newTrio(t)
	ctx := home
	q := store.New(tr.conn)
	nov2026 := civil.Month{Year: 2026, Month: time.November}
	bills := bill.NewService(tr.conn)
	b, err := bills.Create(ctx, tr.people["ana"].ID, bill.Input{Name: "Internet", CategoryID: tr.cat("Energia"), AccountID: tr.accounts["ana"], DueDay: 5, ExpectedCents: 10_000, StartMonth: civil.Month{Year: 2026, Month: time.October}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bills.Pay(ctx, tr.people["ana"].ID, bill.Payment{BillID: b.ID, Month: civil.Month{Year: 2026, Month: time.October}, AmountCents: 10_000, Date: civil.Date{Year: 2026, Month: time.October, Day: 5}}); err != nil {
		t.Fatal(err)
	}
	// From now on the bill is only Ana's; October was paid by the weights and stays so.
	only := &membersplit.Rule{Mode: membersplit.ModeEqual, Members: []membersplit.Member{{PersonID: tr.person("ana")}}}
	if err := membersplit.Save(context.Background(), q, 1, membersplit.KindBill, b.ID, only, 10_000, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	// Saving the same division again records nothing new.
	if err := membersplit.Save(context.Background(), q, 1, membersplit.KindBill, b.ID, only, 10_000, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	var rows int64
	_ = tr.conn.QueryRow(`SELECT count(*) FROM member_splits WHERE item_kind = 'bill' AND item_id = ?`, b.ID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("divisions recorded = %d, want 1", rows)
	}
	if _, err := bills.Pay(ctx, tr.people["ana"].ID, bill.Payment{BillID: b.ID, Month: nov2026, AmountCents: 10_000, Date: nov(5)}); err != nil {
		t.Fatal(err)
	}
	// The October payment, before the change, keeps the weights of the three members.
	st, err := tr.s.Settlement(ctx, civil.Month{Year: 2026, Month: time.October})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Lines) != 1 || len(st.Lines[0].Shares) != 3 {
		t.Fatalf("October payment = %+v, want the weights of the three members", st.Lines)
	}

	// Someone who joins after an expense was recorded, on its very day, takes no part in it.
	dani := testutil.NewUser(t, tr.conn, "dani", "Dani Ficticia")
	before := time.Now().UTC().Add(-time.Hour)
	if _, err := tr.conn.Exec(`INSERT INTO group_weights (group_id, person_id, weight_bp, from_day, set_at) VALUES (1, ?, 2500, '2026-11-12', ?)`, dani.PersonID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.conn.Exec(`INSERT INTO group_members (group_id, person_id, role, weight_bp, joined_at) VALUES (1, ?, 'member', 2500, ?)`, dani.PersonID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	e, err := tr.ledger.AddExpense(ctx, tr.people["ana"].ID, ledger.NewExpense{AccountID: tr.accounts["ana"], AmountCents: 9_000, Date: nov(12), CategoryID: tr.cat("Delivery"), Description: "Antes da Dani"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.conn.Exec(`UPDATE entries SET created_at = ? WHERE id = ?`, before, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.ledger.AddExpense(ctx, tr.people["ana"].ID, ledger.NewExpense{AccountID: tr.accounts["ana"], AmountCents: 9_000, Date: nov(12), CategoryID: tr.cat("Delivery"), Description: "Depois da Dani"}); err != nil {
		t.Fatal(err)
	}
	st, err = tr.s.Settlement(ctx, nov2026)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range st.Lines {
		took := false
		for _, sh := range l.Shares {
			took = took || sh.PersonID == dani.PersonID
		}
		if l.Description == "Antes da Dani" && took {
			t.Errorf("Dani took part in an expense recorded before she joined: %+v", l)
		}
		if l.Description == "Depois da Dani" && !took {
			t.Errorf("Dani took no part in an expense recorded after she joined: %+v", l)
		}
	}
}

// TestPurchasesBeforeTheStartStayOut leaves out every installment of a card purchase made before the settlement
// started, even when its invoices close after it (RN-31).
func TestPurchasesBeforeTheStartStayOut(t *testing.T) {
	tr := newTrio(t)
	c, err := tr.cards.CreateCard(home, tr.people["carla"].ID, card.CardInput{Name: "Cartao Carla", PersonID: tr.person("carla"), LimitCents: 1_000_000, ClosingDay: 3, DueDay: 10, Color: "purple", Finish: "matte"})
	if err != nil {
		t.Fatal(err)
	}
	// Bought on September 20, before the start on October 1, in 3x: October, November and December invoices.
	if _, err := tr.cards.AddPurchase(home, tr.people["carla"].ID, card.PurchaseInput{CardID: c.ID, Date: civil.Date{Year: 2026, Month: time.September, Day: 20}, Merchant: "Ar-condicionado", CategoryID: tr.cat("Delivery"), AmountCents: 300_000, Installments: 3}); err != nil {
		t.Fatal(err)
	}
	st, err := tr.s.Settlement(home, civil.Month{Year: 2026, Month: time.November})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range st.Members {
		if m.PaidCents != 0 || m.ShareCents != 0 {
			t.Fatalf("a purchase from before the start entered the settlement: %+v", st.Members)
		}
	}
}
