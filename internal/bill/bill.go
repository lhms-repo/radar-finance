// Package bill manages fixed bills paid straight from an account, with the
// expected and the real amount of each month (RF-35 to RF-37, RN-14).
package bill

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// maxName is the longest bill name accepted, in characters.
const maxName = 60

// Business errors returned by the service.
var (
	ErrNotFound       = problem.NotFound("bill_not_found", "Conta fixa não encontrada.")
	ErrNameRequired   = problem.Invalid("bill_name_required", "Informe o nome da conta.")
	ErrNameTooLong    = problem.Invalid("bill_name_too_long", "Use no máximo %d caracteres no nome.", maxName)
	ErrInvalidDay     = problem.Invalid("bill_day_invalid", "Informe um dia de vencimento entre 1 e 31.")
	ErrStartRequired  = problem.Invalid("bill_start_required", "Informe o primeiro mês da conta.")
	ErrEndBeforeStart = problem.Invalid("bill_end_before_start", "O último mês é anterior ao primeiro.")
	ErrNoOccurrence   = problem.Invalid("bill_no_occurrence", "Esta conta não vence no mês escolhido.")
	ErrAlreadyPaid    = problem.Conflict("bill_already_paid", "Esta conta já foi paga neste mês.")
	ErrInUse          = problem.Conflict("bill_in_use", "Esta conta já tem pagamentos. Informe o último mês para encerrá-la.")
	ErrPaidOutside    = problem.Conflict("bill_paid_outside", "Esta mudança tiraria da conta um mês já pago. Apague o pagamento desse mês antes de mudar os meses.")
)

// Bill is a registered fixed bill.
type Bill struct {
	ID                 int64
	Name               string
	CategoryID         int64
	CategoryName       string
	ParentCategoryName string
	CategoryIcon       string
	AccountID          int64
	AccountName        string
	DueDay             int
	ExpectedCents      int64
	StartMonth         civil.Month
	EndMonth           *civil.Month
}

// Input creates or replaces a bill (RF-35).
type Input struct {
	Name          string
	CategoryID    int64
	AccountID     int64
	DueDay        int
	ExpectedCents int64
	StartMonth    civil.Month
	EndMonth      *civil.Month
	// Members divides the expense between the members of the group; nil keeps the weights (RN-28).
	Members *membersplit.Rule
}

// Occurrence is the bill of one month, with the amount that counts and the payment, if any.
type Occurrence struct {
	BillID             int64
	Name               string
	CategoryID         int64
	CategoryName       string
	ParentCategoryName string
	CategoryIcon       string
	AccountID          int64
	AccountName        string
	DueOn              civil.Date
	ExpectedCents      int64
	ActualCents        *int64
	// AmountCents is the real amount when informed, otherwise the expected one (RN-14).
	AmountCents int64
	Paid        bool
	EntryID     *int64
	PaidOn      *civil.Date
	PaidCents   int64
	PaidFrom    string
}

// MonthView lists the bills of a month with their totals.
type MonthView struct {
	Month      civil.Month
	Bills      []Occurrence
	TotalCents int64
	PaidCents  int64
	PaidCount  int
}

// Payment pays the bill of a month; zero fields take the amount of the month, the bill account and today.
type Payment struct {
	BillID      int64
	Month       civil.Month
	AmountCents int64
	AccountID   int64
	Date        civil.Date
}

// Service reads and writes fixed bills.
type Service struct {
	conn   *sql.DB
	ledger *ledger.Service
	now    func() time.Time
	today  func() civil.Date
}

// NewService creates the bill service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, ledger: ledger.NewService(conn), now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// List returns every bill of the active group ordered by due day.
func (s *Service) List(ctx context.Context) ([]Bill, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.conn).ListBills(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list bills: %w", err)
	}
	out := make([]Bill, 0, len(rows))
	for _, r := range rows {
		b, err := fromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// Get returns one bill.
func (s *Service) Get(ctx context.Context, id int64) (Bill, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Bill{}, err
	}
	for _, b := range all {
		if b.ID == id {
			return b, nil
		}
	}
	return Bill{}, ErrNotFound
}

// Create registers a bill (RF-35).
func (s *Service) Create(ctx context.Context, userID int64, in Input) (Bill, error) {
	in, err := validate(in)
	if err != nil {
		return Bill{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Bill{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkRefs(ctx, q, in); err != nil {
			return err
		}
		now := s.now().UTC()
		b, err := q.CreateBill(ctx, store.CreateBillParams{
			Name: in.Name, CategoryID: in.CategoryID, AccountID: in.AccountID, DueDay: int64(in.DueDay), ExpectedCents: in.ExpectedCents,
			StartMonth: in.StartMonth.String(), EndMonth: nullMonth(in.EndMonth), CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("create bill: %w", err)
		}
		id = b.ID
		return membersplit.Save(ctx, q, groupID, membersplit.KindBill, id, in.Members, in.ExpectedCents, s.now(), true)
	})
	if err != nil {
		return Bill{}, err
	}
	return s.Get(ctx, id)
}

// Update replaces the fields of a bill; months with a real amount keep it.
func (s *Service) Update(ctx context.Context, id int64, in Input) (Bill, error) {
	in, err := validate(in)
	if err != nil {
		return Bill{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Bill{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getBill(ctx, q, id); err != nil {
			return err
		}
		if err := checkRefs(ctx, q, in); err != nil {
			return err
		}
		if err := checkPaidMonths(ctx, q, id, in); err != nil {
			return err
		}
		if err := q.UpdateBill(ctx, store.UpdateBillParams{
			Name: in.Name, CategoryID: in.CategoryID, AccountID: in.AccountID, DueDay: int64(in.DueDay), ExpectedCents: in.ExpectedCents,
			StartMonth: in.StartMonth.String(), EndMonth: nullMonth(in.EndMonth), UpdatedAt: s.now().UTC(), ID: id, GroupID: groupID,
		}); err != nil {
			return err
		}
		return membersplit.Save(ctx, q, groupID, membersplit.KindBill, id, in.Members, in.ExpectedCents, s.now(), false)
	})
	if err != nil {
		return Bill{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes a bill without payments.
func (s *Service) Delete(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getBill(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteBill(ctx, store.DeleteBillParams{ID: id, GroupID: groupID}); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrInUse
			}
			return fmt.Errorf("delete bill: %w", err)
		}
		return nil
	})
}

// SetActual informs the real amount of a month; zero clears it and the expected amount counts again (RF-36, RN-14).
func (s *Service) SetActual(ctx context.Context, billID int64, m civil.Month, cents int64) error {
	if cents < 0 {
		return ledger.ErrAmountRequired
	}
	if cents > ledger.MaxAmountCents {
		return ledger.ErrAmountTooLarge
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		b, err := getBill(ctx, q, billID)
		if err != nil {
			return err
		}
		if _, ok := dueOn(b, m); !ok {
			return ErrNoOccurrence
		}
		// Once paid, the paid amount is the real one of the month (RF-37).
		paid, err := q.ListBillPayments(ctx, store.ListBillPaymentsParams{GroupID: groupID, BillMonth: sql.NullString{String: m.String(), Valid: true}})
		if err != nil {
			return err
		}
		for _, p := range paid {
			if p.BillID.Int64 == billID {
				return ErrAlreadyPaid
			}
		}
		if cents == 0 {
			return q.DeleteBillAmount(ctx, store.DeleteBillAmountParams{BillID: billID, Month: m.String()})
		}
		return q.UpsertBillAmount(ctx, store.UpsertBillAmountParams{BillID: billID, Month: m.String(), ActualCents: cents})
	})
}

// Month lists the bills due in the month with the amount that counts and the payment state (RF-36, RF-37).
func (s *Service) Month(ctx context.Context, m civil.Month) (MonthView, error) {
	bills, err := s.List(ctx)
	if err != nil {
		return MonthView{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return MonthView{}, err
	}
	q := store.New(s.conn)
	amounts, err := q.ListBillAmounts(ctx, store.ListBillAmountsParams{GroupID: groupID, Month: m.String()})
	if err != nil {
		return MonthView{}, fmt.Errorf("list bill amounts: %w", err)
	}
	actual := map[int64]int64{}
	for _, a := range amounts {
		actual[a.BillID] = a.ActualCents
	}
	payments, err := q.ListBillPayments(ctx, store.ListBillPaymentsParams{GroupID: groupID, BillMonth: sql.NullString{String: m.String(), Valid: true}})
	if err != nil {
		return MonthView{}, fmt.Errorf("list bill payments: %w", err)
	}
	paid := map[int64]store.ListBillPaymentsRow{}
	for _, p := range payments {
		paid[p.BillID.Int64] = p
	}
	view := MonthView{Month: m, Bills: []Occurrence{}}
	for _, b := range bills {
		due, ok := dueOn(b, m)
		if !ok {
			continue
		}
		o := Occurrence{
			BillID: b.ID, Name: b.Name, CategoryID: b.CategoryID, CategoryName: b.CategoryName, ParentCategoryName: b.ParentCategoryName,
			CategoryIcon: b.CategoryIcon, AccountID: b.AccountID, AccountName: b.AccountName, DueOn: due,
			ExpectedCents: b.ExpectedCents, AmountCents: b.ExpectedCents,
		}
		if a, ok := actual[b.ID]; ok {
			o.ActualCents, o.AmountCents = &a, a
		}
		if p, ok := paid[b.ID]; ok {
			on, err := civil.ParseDate(p.OccurredOn)
			if err != nil {
				return MonthView{}, err
			}
			o.Paid, o.EntryID, o.PaidOn, o.PaidCents, o.PaidFrom = true, &p.ID, &on, -p.AmountCents, p.AccountName
			view.PaidCents += -p.AmountCents
			view.PaidCount++
		}
		view.TotalCents += o.AmountCents
		view.Bills = append(view.Bills, o)
	}
	return view, nil
}

// Pay records the payment of the bill of a month as spending of its category; the amount paid becomes the real one (RF-37).
func (s *Service) Pay(ctx context.Context, userID int64, p Payment) (ledger.Entry, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return ledger.Entry{}, err
	}
	var entryID int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		b, err := getBill(ctx, q, p.BillID)
		if err != nil {
			return err
		}
		if _, ok := dueOn(b, p.Month); !ok {
			return ErrNoOccurrence
		}
		done, err := q.ListBillPayments(ctx, store.ListBillPaymentsParams{GroupID: groupID, BillMonth: sql.NullString{String: p.Month.String(), Valid: true}})
		if err != nil {
			return err
		}
		for _, d := range done {
			if d.BillID.Int64 == b.ID {
				return ErrAlreadyPaid
			}
		}
		amount := p.AmountCents
		if amount == 0 {
			amount = b.ExpectedCents
			amounts, err := q.ListBillAmounts(ctx, store.ListBillAmountsParams{GroupID: groupID, Month: p.Month.String()})
			if err != nil {
				return err
			}
			for _, a := range amounts {
				if a.BillID == b.ID {
					amount = a.ActualCents
				}
			}
		}
		if err := ledger.CheckPositive(amount); err != nil {
			return err
		}
		accountID, on := p.AccountID, p.Date
		if accountID == 0 {
			accountID = b.AccountID
		}
		if on.IsZero() {
			on = s.today()
		}
		if err := ledger.CheckActiveAccount(ctx, q, accountID); err != nil {
			return err
		}
		if err := category.CheckSelectable(ctx, q, b.CategoryID); err != nil {
			return err
		}
		if err := q.UpsertBillAmount(ctx, store.UpsertBillAmountParams{BillID: b.ID, Month: p.Month.String(), ActualCents: amount}); err != nil {
			return err
		}
		now := s.now().UTC()
		entryID, err = q.CreateBillEntry(ctx, store.CreateBillEntryParams{
			AccountID: accountID, AmountCents: -amount, OccurredOn: on.String(),
			CategoryID: sql.NullInt64{Int64: b.CategoryID, Valid: true}, Description: b.Name,
			BillID: sql.NullInt64{Int64: b.ID, Valid: true}, BillMonth: sql.NullString{String: p.Month.String(), Valid: true},
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now, GroupID: groupID,
		})
		if err != nil {
			return fmt.Errorf("pay bill: %w", err)
		}
		return nil
	})
	if err != nil {
		return ledger.Entry{}, err
	}
	return s.ledger.Entry(ctx, entryID)
}

// dueOn returns the due date of the bill in month m, if the bill runs that month.
func dueOn(b Bill, m civil.Month) (civil.Date, bool) {
	if monthIndex(m) < monthIndex(b.StartMonth) || (b.EndMonth != nil && monthIndex(m) > monthIndex(*b.EndMonth)) {
		return civil.Date{}, false
	}
	return m.Day(b.DueDay), true
}

// checkPaidMonths refuses a change that leaves a paid month outside the bill, which would hide the payment from the forecast.
func checkPaidMonths(ctx context.Context, q *store.Queries, id int64, in Input) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	months, err := q.ListBillPaidMonths(ctx, store.ListBillPaidMonthsParams{GroupID: groupID, BillID: sql.NullInt64{Int64: id, Valid: true}})
	if err != nil {
		return err
	}
	next := Bill{DueDay: in.DueDay, StartMonth: in.StartMonth, EndMonth: in.EndMonth}
	for _, s := range months {
		m, err := civil.ParseMonth(s)
		if err != nil {
			return err
		}
		if _, ok := dueOn(next, m); !ok {
			return ErrPaidOutside
		}
	}
	return nil
}

// validate trims and checks the bill fields.
func validate(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return in, ErrNameRequired
	case utf8.RuneCountInString(in.Name) > maxName:
		return in, ErrNameTooLong
	case in.DueDay < 1 || in.DueDay > 31:
		return in, ErrInvalidDay
	case in.StartMonth == civil.Month{}:
		return in, ErrStartRequired
	case in.EndMonth != nil && monthIndex(*in.EndMonth) < monthIndex(in.StartMonth):
		return in, ErrEndBeforeStart
	}
	return in, ledger.CheckPositive(in.ExpectedCents)
}

// checkRefs ensures the category classifies entries and the account accepts them.
func checkRefs(ctx context.Context, q *store.Queries, in Input) error {
	if err := category.CheckSelectable(ctx, q, in.CategoryID); err != nil {
		return err
	}
	return ledger.CheckActiveAccount(ctx, q, in.AccountID)
}

// getBill reads one bill with its display names.
func getBill(ctx context.Context, q *store.Queries, id int64) (Bill, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Bill{}, err
	}
	r, err := q.GetBill(ctx, store.GetBillParams{ID: id, GroupID: groupID})
	if errors.Is(err, sql.ErrNoRows) {
		return Bill{}, ErrNotFound
	}
	if err != nil {
		return Bill{}, err
	}
	return fromRow(store.ListBillsRow{
		ID: r.ID, Name: r.Name, CategoryID: r.CategoryID, AccountID: r.AccountID, DueDay: r.DueDay, ExpectedCents: r.ExpectedCents,
		StartMonth: r.StartMonth, EndMonth: r.EndMonth,
	})
}

// fromRow converts a stored bill.
func fromRow(r store.ListBillsRow) (Bill, error) {
	start, err := civil.ParseMonth(r.StartMonth)
	if err != nil {
		return Bill{}, err
	}
	b := Bill{
		ID: r.ID, Name: r.Name, CategoryID: r.CategoryID, CategoryName: r.CategoryName, ParentCategoryName: r.ParentCategoryName,
		CategoryIcon: r.CategoryIcon, AccountID: r.AccountID, AccountName: r.AccountName, DueDay: int(r.DueDay),
		ExpectedCents: r.ExpectedCents, StartMonth: start,
	}
	if r.EndMonth.Valid {
		end, err := civil.ParseMonth(r.EndMonth.String)
		if err != nil {
			return Bill{}, err
		}
		b.EndMonth = &end
	}
	return b, nil
}

// nullMonth stores an optional month as text.
func nullMonth(m *civil.Month) sql.NullString {
	if m == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: m.String(), Valid: true}
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
