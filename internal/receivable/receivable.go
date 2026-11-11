// Package receivable lists what other people owe from split purchases, records
// what they pay back and writes the charge message (RF-26, RF-28 to RF-30, RN-06).
package receivable

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/money"
	"github.com/italogfx/radar-finance/internal/problem"
)

// KindReceivable is the entry kind of a share paid back; it is not income (RN-09).
const KindReceivable ledger.Kind = "receivable"

// Business errors returned by the service.
var (
	ErrNothingChosen   = problem.Invalid("receivable_nothing", "Escolha o que foi pago.")
	ErrMixedPeople     = problem.Invalid("receivable_mixed", "Marque os pagamentos de uma pessoa por vez.")
	ErrItemNotFound    = problem.NotFound("receivable_not_found", "Cota não encontrada.")
	ErrAlreadyReceived = problem.Conflict("receivable_received", "Esta cota já foi recebida.")
	ErrPartyNotFound   = problem.NotFound("party_not_found", "Pessoa não encontrada.")
)

// Item is the share of a person in one installment of a split purchase.
type Item struct {
	ID           int64
	PurchaseID   int64
	Merchant     string
	PurchaseDate civil.Date
	CardName     string
	Number       int
	Count        int
	InvoiceMonth civil.Month
	// DueOn is the due date of the invoice that carries this installment.
	DueOn       civil.Date
	AmountCents int64
	Received    bool
	EntryID     *int64
	ReceivedOn  *civil.Date
	ReceivedIn  string
}

// Person groups what one person owes, with the pending total (RF-28).
type Person struct {
	PartyID      int64
	Name         string
	PendingCents int64
	Items        []Item
}

// Receipt marks shares of one person as paid back into an account (RF-26, RF-30).
type Receipt struct {
	ItemIDs   []int64
	AccountID int64
	Date      civil.Date
}

// Service reads and records what other people owe.
type Service struct {
	conn   *sql.DB
	ledger *ledger.Service
	now    func() time.Time
}

// NewService creates the receivables service.
func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn, ledger: ledger.NewService(conn), now: time.Now}
}

// List returns everyone with shares, pending and received, grouped by person in name order (RF-28).
func (s *Service) List(ctx context.Context) ([]Person, error) {
	rows, err := store.New(s.conn).ListReceivables(ctx)
	if err != nil {
		return nil, fmt.Errorf("list receivables: %w", err)
	}
	var out []Person
	for _, r := range rows {
		if len(out) == 0 || out[len(out)-1].PartyID != r.PartyID {
			out = append(out, Person{PartyID: r.PartyID, Name: r.PartyName, Items: []Item{}})
		}
		p := &out[len(out)-1]
		it, err := toItem(r)
		if err != nil {
			return nil, err
		}
		p.Items = append(p.Items, it)
		if !it.Received {
			p.PendingCents += it.AmountCents
		}
	}
	if out == nil {
		out = []Person{}
	}
	return out, nil
}

// MarkReceived records the shares of one person paid together as a single entry in the account (RF-30, RN-06).
func (s *Service) MarkReceived(ctx context.Context, userID int64, r Receipt) (ledger.Entry, error) {
	if len(r.ItemIDs) == 0 {
		return ledger.Entry{}, ErrNothingChosen
	}
	var entryID int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		rows, err := q.ListReceivables(ctx)
		if err != nil {
			return err
		}
		byID := map[int64]store.ListReceivablesRow{}
		for _, row := range rows {
			byID[row.ID] = row
		}
		var total int64
		var party int64
		var name string
		var merchants []string
		for _, id := range r.ItemIDs {
			row, ok := byID[id]
			if !ok {
				return ErrItemNotFound
			}
			if row.EntryID.Valid {
				return ErrAlreadyReceived
			}
			if party != 0 && row.PartyID != party {
				return ErrMixedPeople
			}
			party, name = row.PartyID, row.PartyName
			total += row.AmountCents
			if !contains(merchants, row.Merchant) {
				merchants = append(merchants, row.Merchant)
			}
		}
		if r.Date.IsZero() {
			return ledger.ErrDateRequired
		}
		if err := ledger.CheckActiveAccount(ctx, q, r.AccountID); err != nil {
			return err
		}
		now := s.now().UTC()
		desc := truncate(fmt.Sprintf("%s: %s", name, strings.Join(merchants, ", ")), 120)
		e, err := q.CreateEntry(ctx, store.CreateEntryParams{
			AccountID: r.AccountID, Kind: string(KindReceivable), AmountCents: total, OccurredOn: r.Date.String(), Confirmed: true,
			Description: desc, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create receivable entry: %w", err)
		}
		entryID = e.ID
		for _, id := range r.ItemIDs {
			if err := q.SetSplitInstallmentEntry(ctx, store.SetSplitInstallmentEntryParams{EntryID: sql.NullInt64{Int64: e.ID, Valid: true}, ID: id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ledger.Entry{}, err
	}
	return s.ledger.Entry(ctx, entryID)
}

// Message writes the charge for a person, with each pending installment and the total, ready for WhatsApp (RF-29).
func (s *Service) Message(ctx context.Context, partyID int64) (string, error) {
	people, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range people {
		if p.PartyID != partyID {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Oi, %s! Segue o que ficou das compras que dividimos:\n\n", firstName(p.Name))
		for _, it := range p.Items {
			if it.Received {
				continue
			}
			line := it.Merchant
			if it.Count > 1 {
				line += fmt.Sprintf(", parcela %d de %d", it.Number, it.Count)
			}
			fmt.Fprintf(&b, "• %s, fatura de %02d/%d: %s\n", line, it.InvoiceMonth.Month, it.InvoiceMonth.Year, FormatBRL(it.AmountCents))
		}
		fmt.Fprintf(&b, "\nTotal: %s\n\nObrigado!", FormatBRL(p.PendingCents))
		return b.String(), nil
	}
	return "", ErrPartyNotFound
}

// FormatBRL writes cents as Brazilian reais, like R$ 1.234,56, for messages sent out of the app.
func FormatBRL(cents int64) string { return money.Format(cents) }

// toItem converts a stored share, working out the due date of its invoice.
func toItem(r store.ListReceivablesRow) (Item, error) {
	m, err := civil.ParseMonth(r.InvoiceMonth)
	if err != nil {
		return Item{}, err
	}
	on, err := civil.ParseDate(r.PurchasedOn)
	if err != nil {
		return Item{}, err
	}
	cycle := card.Cycle{ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay)}
	it := Item{
		ID: r.ID, PurchaseID: r.PurchaseID, Merchant: r.Merchant, PurchaseDate: on, CardName: r.CardName, Number: int(r.Number),
		Count: int(r.InstallmentCount), InvoiceMonth: m, DueOn: cycle.Due(m), AmountCents: r.AmountCents, Received: r.EntryID.Valid,
	}
	if r.EntryID.Valid {
		id := r.EntryID.Int64
		received, err := civil.ParseDate(r.ReceivedOn)
		if err != nil {
			return Item{}, err
		}
		it.EntryID, it.ReceivedOn, it.ReceivedIn = &id, &received, r.ReceivedIn
	}
	return it, nil
}

// firstName returns the first word of a name, for a friendly greeting.
func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

// contains reports whether list has s.
func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
