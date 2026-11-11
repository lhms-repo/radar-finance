// Package financing manages financings, their monthly installments as bills
// to pay and the outstanding balance informed by the user (RF-39 to RF-42, RN-16).
package financing

import (
	"context"
	"database/sql"
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
	"github.com/italogfx/radar-finance/internal/problem"
)

// Limits of the financing fields.
const (
	maxName         = 60
	maxInstallments = 600
)

// Event kinds of the financing history.
const (
	eventPayment    = "payment"
	eventCorrection = "correction"
)

// Business errors returned by the service.
var (
	ErrNotFound       = problem.NotFound("financing_not_found", "Financiamento não encontrado.")
	ErrNameRequired   = problem.Invalid("financing_name_required", "Informe o nome do financiamento.")
	ErrNameTooLong    = problem.Invalid("financing_name_too_long", "Use no máximo %d caracteres no nome.", maxName)
	ErrInvalidDay     = problem.Invalid("financing_day_invalid", "Informe um dia de vencimento entre 1 e 31.")
	ErrInvalidTotal   = problem.Invalid("financing_total_invalid", "Informe de 1 a %d parcelas no total.", maxInstallments)
	ErrInvalidPaid    = problem.Invalid("financing_paid_invalid", "As parcelas pagas não podem passar do total.")
	ErrInvalidBalance = problem.Invalid("financing_balance_invalid", "Informe um saldo devedor de zero para cima.")
	ErrStartRequired  = problem.Invalid("financing_start_required", "Informe o mês da próxima parcela.")
	ErrNoOccurrence   = problem.Invalid("financing_no_occurrence", "Não há parcela deste financiamento no mês escolhido.")
	ErrAlreadyPaid    = problem.Conflict("financing_already_paid", "A parcela deste mês já foi paga.")
	ErrInUse          = problem.Conflict("financing_in_use", "Este financiamento já tem pagamentos registrados e não pode ser excluído.")
)

// Financing is a registered financing with its current state (RF-41).
type Financing struct {
	ID                    int64
	Name                  string
	CategoryID            int64
	CategoryName          string
	CategoryIcon          string
	AccountID             int64
	AccountName           string
	DueDay                int
	InstallmentCents      int64
	TotalInstallments     int
	PaidInstallments      int
	RemainingInstallments int
	BalanceCents          int64
	StartMonth            civil.Month
	// NextMonth is the month of the first unpaid installment; nil when paid off.
	NextMonth *civil.Month
	Done      bool
	initial   int
	paid      map[civil.Month]event
}

// event is a payment of the history, keyed by month.
type event struct {
	entryID int64
	cents   int64
	on      civil.Date
	from    string
	after   int64
}

// Input registers a financing with its state on the day (RF-39).
type Input struct {
	Name              string
	CategoryID        int64
	AccountID         int64
	DueDay            int
	InstallmentCents  int64
	TotalInstallments int
	PaidInstallments  int
	BalanceCents      int64
	// NextMonth is the month of the next installment to pay.
	NextMonth civil.Month
}

// Changes edits the descriptive fields; amounts change through Correct (RF-42).
type Changes struct {
	Name       string
	CategoryID int64
	AccountID  int64
	DueDay     int
}

// Occurrence is the installment of one month as a bill to pay (RF-40).
type Occurrence struct {
	FinancingID  int64
	Name         string
	CategoryName string
	CategoryIcon string
	AccountID    int64
	AccountName  string
	Number       int
	Total        int
	DueOn        civil.Date
	AmountCents  int64
	Paid         bool
	EntryID      *int64
	PaidOn       *civil.Date
	PaidCents    int64
	PaidFrom     string
	// SuggestedBalanceCents is the balance minus the installment, offered when paying (RN-16).
	SuggestedBalanceCents int64
}

// Payment pays the installment of a month; zero fields take the installment, the financing account and today.
type Payment struct {
	FinancingID       int64
	Month             civil.Month
	AmountCents       int64
	AccountID         int64
	Date              civil.Date
	BalanceAfterCents *int64
}

// Correction sets the balance and the installment by hand, for interest, inflation or prepayment (RF-42).
type Correction struct {
	FinancingID      int64
	BalanceCents     int64
	InstallmentCents int64
}

// Service reads and writes financings.
type Service struct {
	conn   *sql.DB
	ledger *ledger.Service
	now    func() time.Time
	today  func() civil.Date
}

// NewService creates the financing service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, ledger: ledger.NewService(conn), now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// List returns every financing with paid and remaining installments and the current balance (RF-41).
func (s *Service) List(ctx context.Context) ([]Financing, error) {
	return s.list(ctx, store.New(s.conn))
}

// Get returns one financing.
func (s *Service) Get(ctx context.Context, id int64) (Financing, error) {
	return s.get(ctx, store.New(s.conn), id)
}

// Create registers a financing (RF-39).
func (s *Service) Create(ctx context.Context, userID int64, in Input) (Financing, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validate(in); err != nil {
		return Financing{}, err
	}
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkRefs(ctx, q, in.CategoryID, in.AccountID); err != nil {
			return err
		}
		now := s.now().UTC()
		f, err := q.CreateFinancing(ctx, store.CreateFinancingParams{
			Name: in.Name, CategoryID: in.CategoryID, AccountID: in.AccountID, DueDay: int64(in.DueDay),
			InstallmentCents: in.InstallmentCents, TotalInstallments: int64(in.TotalInstallments), InitialPaid: int64(in.PaidInstallments),
			InitialBalanceCents: in.BalanceCents, StartMonth: in.NextMonth.String(), CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create financing: %w", err)
		}
		id = f.ID
		return nil
	})
	if err != nil {
		return Financing{}, err
	}
	return s.Get(ctx, id)
}

// Update edits name, category, account and due day.
func (s *Service) Update(ctx context.Context, id int64, c Changes) (Financing, error) {
	c.Name = strings.TrimSpace(c.Name)
	if err := validateName(c.Name); err != nil {
		return Financing{}, err
	}
	if c.DueDay < 1 || c.DueDay > 31 {
		return Financing{}, ErrInvalidDay
	}
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := s.get(ctx, q, id); err != nil {
			return err
		}
		if err := checkRefs(ctx, q, c.CategoryID, c.AccountID); err != nil {
			return err
		}
		return q.UpdateFinancing(ctx, store.UpdateFinancingParams{
			Name: c.Name, CategoryID: c.CategoryID, AccountID: c.AccountID, DueDay: int64(c.DueDay), UpdatedAt: s.now().UTC(), ID: id,
		})
	})
	if err != nil {
		return Financing{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes a financing without payments.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		f, err := s.get(ctx, q, id)
		if err != nil {
			return err
		}
		if len(f.paid) > 0 {
			return ErrInUse
		}
		return q.DeleteFinancing(ctx, id)
	})
}

// Month lists the installments due in the month as bills to pay (RF-40).
func (s *Service) Month(ctx context.Context, m civil.Month) ([]Occurrence, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []Occurrence{}
	for _, f := range all {
		number, ok := f.numberIn(m)
		if !ok {
			continue
		}
		o := Occurrence{
			FinancingID: f.ID, Name: f.Name, CategoryName: f.CategoryName, CategoryIcon: f.CategoryIcon, AccountID: f.AccountID,
			AccountName: f.AccountName, Number: number, Total: f.TotalInstallments, DueOn: m.Day(f.DueDay), AmountCents: f.InstallmentCents,
			SuggestedBalanceCents: max(0, f.BalanceCents-f.InstallmentCents),
		}
		if e, done := f.paid[m]; done {
			on := e.on
			o.Paid, o.EntryID, o.PaidOn, o.PaidCents, o.PaidFrom, o.AmountCents = true, &e.entryID, &on, e.cents, e.from, e.cents
			o.SuggestedBalanceCents = e.after
		}
		out = append(out, o)
	}
	return out, nil
}

// Pay records the installment as spending of the financing category and sets the balance after it (RF-41, RN-16).
func (s *Service) Pay(ctx context.Context, userID int64, p Payment) (ledger.Entry, error) {
	var entryID int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		f, err := s.get(ctx, q, p.FinancingID)
		if err != nil {
			return err
		}
		number, ok := f.numberIn(p.Month)
		if !ok {
			return ErrNoOccurrence
		}
		if _, done := f.paid[p.Month]; done {
			return ErrAlreadyPaid
		}
		amount, accountID, on := p.AmountCents, p.AccountID, p.Date
		if amount == 0 {
			amount = f.InstallmentCents
		}
		if accountID == 0 {
			accountID = f.AccountID
		}
		if on.IsZero() {
			on = s.today()
		}
		balance := max(0, f.BalanceCents-amount)
		if p.BalanceAfterCents != nil {
			balance = *p.BalanceAfterCents
		}
		if balance < 0 || balance > ledger.MaxAmountCents {
			return ErrInvalidBalance
		}
		if err := ledger.CheckPositive(amount); err != nil {
			return err
		}
		if err := checkRefs(ctx, q, f.CategoryID, accountID); err != nil {
			return err
		}
		now := s.now().UTC()
		e, err := q.CreateEntry(ctx, store.CreateEntryParams{
			AccountID: accountID, Kind: string(ledger.KindExpense), AmountCents: -amount, OccurredOn: on.String(), Confirmed: true,
			CategoryID: sql.NullInt64{Int64: f.CategoryID, Valid: true}, Description: fmt.Sprintf("%s, parcela %d de %d", f.Name, number, f.TotalInstallments),
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create installment entry: %w", err)
		}
		entryID = e.ID
		return q.CreateFinancingEvent(ctx, store.CreateFinancingEventParams{
			FinancingID: f.ID, Kind: eventPayment, Month: sql.NullString{String: p.Month.String(), Valid: true},
			Number: sql.NullInt64{Int64: int64(number), Valid: true}, EntryID: sql.NullInt64{Int64: e.ID, Valid: true},
			BalanceAfterCents: balance, CreatedBy: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return ledger.Entry{}, err
	}
	return s.ledger.Entry(ctx, entryID)
}

// Correct sets the balance and the installment by hand (RF-42).
func (s *Service) Correct(ctx context.Context, userID int64, c Correction) (Financing, error) {
	if c.BalanceCents < 0 || c.BalanceCents > ledger.MaxAmountCents {
		return Financing{}, ErrInvalidBalance
	}
	if err := ledger.CheckPositive(c.InstallmentCents); err != nil {
		return Financing{}, err
	}
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := s.get(ctx, q, c.FinancingID); err != nil {
			return err
		}
		now := s.now().UTC()
		if err := q.SetFinancingInstallment(ctx, store.SetFinancingInstallmentParams{InstallmentCents: c.InstallmentCents, UpdatedAt: now, ID: c.FinancingID}); err != nil {
			return err
		}
		return q.CreateFinancingEvent(ctx, store.CreateFinancingEventParams{
			FinancingID: c.FinancingID, Kind: eventCorrection, BalanceAfterCents: c.BalanceCents, CreatedBy: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return Financing{}, err
	}
	return s.Get(ctx, c.FinancingID)
}

// numberIn returns the installment number due in month m, if any.
func (f Financing) numberIn(m civil.Month) (int, bool) {
	offset := monthIndex(m) - monthIndex(f.StartMonth)
	number := f.initial + offset + 1
	if offset < 0 || number > f.TotalInstallments {
		return 0, false
	}
	return number, true
}

// get reads one financing with its state.
func (s *Service) get(ctx context.Context, q *store.Queries, id int64) (Financing, error) {
	all, err := s.list(ctx, q)
	if err != nil {
		return Financing{}, err
	}
	for _, f := range all {
		if f.ID == id {
			return f, nil
		}
	}
	return Financing{}, ErrNotFound
}

// list reads every financing and replays its events to get the paid count and the balance.
func (s *Service) list(ctx context.Context, q *store.Queries) ([]Financing, error) {
	rows, err := q.ListFinancings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list financings: %w", err)
	}
	events, err := q.ListFinancingEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list financing events: %w", err)
	}
	out := make([]Financing, 0, len(rows))
	for _, r := range rows {
		start, err := civil.ParseMonth(r.StartMonth)
		if err != nil {
			return nil, err
		}
		f := Financing{
			ID: r.ID, Name: r.Name, CategoryID: r.CategoryID, CategoryName: r.CategoryName, CategoryIcon: r.CategoryIcon,
			AccountID: r.AccountID, AccountName: r.AccountName, DueDay: int(r.DueDay), InstallmentCents: r.InstallmentCents,
			TotalInstallments: int(r.TotalInstallments), BalanceCents: r.InitialBalanceCents, StartMonth: start,
			initial: int(r.InitialPaid), paid: map[civil.Month]event{},
		}
		for _, e := range events {
			if e.FinancingID != f.ID {
				continue
			}
			f.BalanceCents = e.BalanceAfterCents
			if e.Kind != eventPayment {
				continue
			}
			m, err := civil.ParseMonth(e.Month.String)
			if err != nil {
				return nil, err
			}
			on, err := civil.ParseDate(e.PaidOn)
			if err != nil {
				return nil, err
			}
			f.paid[m] = event{entryID: e.EntryID.Int64, cents: -e.EntryAmountCents, on: on, from: e.PaidFrom, after: e.BalanceAfterCents}
		}
		f.PaidInstallments = f.initial + len(f.paid)
		f.RemainingInstallments = f.TotalInstallments - f.PaidInstallments
		for m := start; ; m = m.Add(1) {
			if _, ok := f.numberIn(m); !ok {
				break
			}
			if _, done := f.paid[m]; !done {
				next := m
				f.NextMonth = &next
				break
			}
		}
		f.Done = f.NextMonth == nil
		out = append(out, f)
	}
	return out, nil
}

// validate checks the fields of a new financing.
func validate(in Input) error {
	if err := validateName(in.Name); err != nil {
		return err
	}
	switch {
	case in.DueDay < 1 || in.DueDay > 31:
		return ErrInvalidDay
	case in.TotalInstallments < 1 || in.TotalInstallments > maxInstallments:
		return ErrInvalidTotal
	case in.PaidInstallments < 0 || in.PaidInstallments > in.TotalInstallments:
		return ErrInvalidPaid
	case in.BalanceCents < 0 || in.BalanceCents > ledger.MaxAmountCents:
		return ErrInvalidBalance
	case in.NextMonth == civil.Month{}:
		return ErrStartRequired
	}
	return ledger.CheckPositive(in.InstallmentCents)
}

// validateName checks a financing name.
func validateName(name string) error {
	if name == "" {
		return ErrNameRequired
	}
	if utf8.RuneCountInString(name) > maxName {
		return ErrNameTooLong
	}
	return nil
}

// checkRefs ensures the category classifies entries and the account accepts them.
func checkRefs(ctx context.Context, q *store.Queries, categoryID, accountID int64) error {
	if err := category.CheckSelectable(ctx, q, categoryID); err != nil {
		return err
	}
	return ledger.CheckActiveAccount(ctx, q, accountID)
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
