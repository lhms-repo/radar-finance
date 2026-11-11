// Package income manages the recurring and eventual incomes of the couple and
// the confirmation of each receipt (RF-09 to RF-13, RN-13).
//
// The planned occurrences of a month are derived from the registered incomes,
// so a change in the amount applies to every month not confirmed yet.
// Confirming writes an income entry, and only then the balance changes.
package income

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
)

// EndingSoonDays is how close the end date must be to warn about it (RF-12).
const EndingSoonDays = 30

// maxName is the longest income name accepted, in characters.
const maxName = 60

// Kind tells whether an income repeats every month or happens once.
type Kind string

// Income kinds (RF-09, RF-11).
const (
	KindRecurring Kind = "recurring"
	KindEventual  Kind = "eventual"
)

// Business errors returned by the service.
var (
	ErrNotFound         = problem.NotFound("income_not_found", "Renda não encontrada.")
	ErrInvalidKind      = problem.Invalid("income_kind_invalid", "Tipo de renda inválido.")
	ErrKindChange       = problem.Invalid("income_kind_change", "Não é possível trocar o tipo de uma renda já cadastrada.")
	ErrNameRequired     = problem.Invalid("income_name_required", "Informe o nome da renda.")
	ErrNameTooLong      = problem.Invalid("income_name_too_long", "Use no máximo %d caracteres no nome.", maxName)
	ErrInvalidDay       = problem.Invalid("income_day_invalid", "Informe um dia de recebimento entre 1 e 31.")
	ErrStartRequired    = problem.Invalid("income_start_required", "Informe o mês do primeiro recebimento.")
	ErrEndBeforeStart   = problem.Invalid("income_end_before_start", "A data de término é anterior ao primeiro recebimento.")
	ErrExpectedRequired = problem.Invalid("income_expected_required", "Informe a data prevista.")
	ErrPersonNotFound   = problem.Invalid("income_person_not_found", "Escolha de quem é a renda.")
	ErrNoOccurrence     = problem.Invalid("income_no_occurrence", "Esta renda não está prevista para o mês escolhido.")
	ErrAlreadyConfirmed = problem.Conflict("income_already_confirmed", "O recebimento deste mês já foi confirmado.")
	ErrInUse            = problem.Conflict("income_in_use", "Esta renda já tem recebimentos confirmados. Informe uma data de término para encerrá-la.")
)

// Income is a registered income with the names resolved for display.
type Income struct {
	ID          int64
	Kind        Kind
	Name        string
	PersonID    int64
	PersonName  string
	AccountID   int64
	AccountName string
	AmountCents int64
	Day         int
	StartMonth  civil.Month
	EndOn       *civil.Date
	ExpectedOn  *civil.Date
	// EndingSoon is true when the end date is within EndingSoonDays (RF-12).
	EndingSoon bool
	Ended      bool
}

// Input creates or replaces an income; recurring ones use Day, StartMonth and EndOn, eventual ones ExpectedOn.
type Input struct {
	Kind        Kind
	Name        string
	PersonID    int64
	AccountID   int64
	AmountCents int64
	Day         int
	StartMonth  civil.Month
	EndOn       *civil.Date
	ExpectedOn  civil.Date
}

// Occurrence is the income expected in a month and, once confirmed, what was received.
type Occurrence struct {
	IncomeID      int64
	Kind          Kind
	Name          string
	PersonID      int64
	PersonName    string
	AccountID     int64
	AccountName   string
	ExpectedOn    civil.Date
	ExpectedCents int64
	Confirmed     bool
	EntryID       *int64
	ReceivedCents int64
	ReceivedOn    *civil.Date
	ReceivedIn    string
}

// OneOff is an income received outside the registered ones (RF-13).
type OneOff struct {
	EntryID int64
	// PersonID is the holder of the account that received it.
	PersonID    int64
	AmountCents int64
	Date        civil.Date
	Description string
	AccountName string
}

// MonthView lists the incomes of a month with what is still expected and what was received.
type MonthView struct {
	Month         civil.Month
	Occurrences   []Occurrence
	OneOffs       []OneOff
	PendingCents  int64
	ReceivedCents int64
}

// Confirmation marks the receipt of an occurrence; zero fields take the expected values.
type Confirmation struct {
	IncomeID    int64
	Month       civil.Month
	AmountCents int64
	Date        civil.Date
	AccountID   int64
}

// Service reads incomes, derives occurrences and confirms receipts.
type Service struct {
	conn   *sql.DB
	ledger *ledger.Service
	now    func() time.Time
	today  func() civil.Date
}

// NewService creates the income service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, ledger: ledger.NewService(conn), now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// List returns every registered income, recurring ones first.
func (s *Service) List(ctx context.Context) ([]Income, error) {
	rows, err := store.New(s.conn).ListIncomes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list incomes: %w", err)
	}
	today := s.today()
	out := make([]Income, 0, len(rows))
	for _, r := range rows {
		inc, err := fromRow(r, today)
		if err != nil {
			return nil, err
		}
		out = append(out, inc)
	}
	return out, nil
}

// Get returns one income.
func (s *Service) Get(ctx context.Context, id int64) (Income, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Income{}, err
	}
	for _, i := range all {
		if i.ID == id {
			return i, nil
		}
	}
	return Income{}, ErrNotFound
}

// Create registers an income (RF-09, RF-11).
func (s *Service) Create(ctx context.Context, userID int64, in Input) (Income, error) {
	in, err := validate(in)
	if err != nil {
		return Income{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkRefs(ctx, q, in); err != nil {
			return err
		}
		now := s.now().UTC()
		p := toParams(in)
		row, err := q.CreateIncome(ctx, store.CreateIncomeParams{
			Kind: string(in.Kind), Name: p.Name, PersonID: p.PersonID, AccountID: p.AccountID, AmountCents: p.AmountCents,
			Day: p.Day, StartMonth: p.StartMonth, EndOn: p.EndOn, ExpectedOn: p.ExpectedOn,
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create income: %w", err)
		}
		id = row.ID
		return nil
	})
	if err != nil {
		return Income{}, err
	}
	return s.Get(ctx, id)
}

// Update replaces the fields of an income; confirmed months keep what was received (RF-12).
func (s *Service) Update(ctx context.Context, id int64, in Input) (Income, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := q.GetIncome(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if Kind(cur.Kind) != in.Kind {
			return ErrKindChange
		}
		if in, err = validate(in); err != nil {
			return err
		}
		if err := checkRefs(ctx, q, in); err != nil {
			return err
		}
		p := toParams(in)
		p.UpdatedAt, p.ID = s.now().UTC(), id
		return q.UpdateIncome(ctx, p)
	})
	if err != nil {
		return Income{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes an income without confirmed receipts.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetIncome(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := q.DeleteIncome(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrInUse
			}
			return fmt.Errorf("delete income: %w", err)
		}
		return nil
	})
}

// Month lists the expected and received incomes of the month (RF-10, RF-13, RN-13).
func (s *Service) Month(ctx context.Context, m civil.Month) (MonthView, error) {
	incomes, err := s.List(ctx)
	if err != nil {
		return MonthView{}, err
	}
	q := store.New(s.conn)
	confirmations, err := q.ListIncomeConfirmations(ctx, sql.NullString{String: m.String(), Valid: true})
	if err != nil {
		return MonthView{}, fmt.Errorf("list confirmations: %w", err)
	}
	byIncome := map[int64]store.ListIncomeConfirmationsRow{}
	for _, c := range confirmations {
		byIncome[c.IncomeID.Int64] = c
	}
	view := MonthView{Month: m, Occurrences: []Occurrence{}, OneOffs: []OneOff{}}
	for _, inc := range incomes {
		on, ok := occurrence(inc, m)
		if !ok {
			continue
		}
		o := Occurrence{
			IncomeID: inc.ID, Kind: inc.Kind, Name: inc.Name, PersonID: inc.PersonID, PersonName: inc.PersonName, AccountID: inc.AccountID,
			AccountName: inc.AccountName, ExpectedOn: on, ExpectedCents: inc.AmountCents,
		}
		if c, done := byIncome[inc.ID]; done {
			d, err := civil.ParseDate(c.OccurredOn)
			if err != nil {
				return MonthView{}, err
			}
			o.Confirmed, o.EntryID, o.ReceivedCents, o.ReceivedOn, o.ReceivedIn = true, &c.ID, c.AmountCents, &d, c.AccountName
			view.ReceivedCents += c.AmountCents
		} else {
			view.PendingCents += inc.AmountCents
		}
		view.Occurrences = append(view.Occurrences, o)
	}
	oneOffs, err := q.ListOneOffIncomes(ctx, store.ListOneOffIncomesParams{FirstDay: m.First().String(), LastDay: m.Last().String()})
	if err != nil {
		return MonthView{}, fmt.Errorf("list one off incomes: %w", err)
	}
	for _, r := range oneOffs {
		d, err := civil.ParseDate(r.OccurredOn)
		if err != nil {
			return MonthView{}, err
		}
		view.OneOffs = append(view.OneOffs, OneOff{EntryID: r.ID, PersonID: r.PersonID, AmountCents: r.AmountCents, Date: d, Description: r.Description, AccountName: r.AccountName})
		view.ReceivedCents += r.AmountCents
	}
	return view, nil
}

// Confirm records the receipt of an occurrence as a confirmed entry (RF-10, RN-13).
func (s *Service) Confirm(ctx context.Context, userID int64, c Confirmation) (ledger.Entry, error) {
	var entryID int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		row, err := q.GetIncome(ctx, c.IncomeID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		inc, err := fromRow(store.ListIncomesRow{
			ID: row.ID, Kind: row.Kind, Name: row.Name, PersonID: row.PersonID, AccountID: row.AccountID, AmountCents: row.AmountCents,
			Day: row.Day, StartMonth: row.StartMonth, EndOn: row.EndOn, ExpectedOn: row.ExpectedOn,
		}, s.today())
		if err != nil {
			return err
		}
		expectedOn, ok := occurrence(inc, c.Month)
		if !ok {
			return ErrNoOccurrence
		}
		done, err := q.ListIncomeConfirmations(ctx, sql.NullString{String: c.Month.String(), Valid: true})
		if err != nil {
			return err
		}
		for _, d := range done {
			if d.IncomeID.Int64 == inc.ID {
				return ErrAlreadyConfirmed
			}
		}
		amount, on, accountID := inc.AmountCents, expectedOn, inc.AccountID
		if c.AmountCents != 0 {
			amount = c.AmountCents
		}
		if !c.Date.IsZero() {
			on = c.Date
		}
		if c.AccountID != 0 {
			accountID = c.AccountID
		}
		if err := ledger.CheckPositive(amount); err != nil {
			return err
		}
		if err := ledger.CheckActiveAccount(ctx, q, accountID); err != nil {
			return err
		}
		now := s.now().UTC()
		entryID, err = q.CreateIncomeEntry(ctx, store.CreateIncomeEntryParams{
			AccountID: accountID, AmountCents: amount, OccurredOn: on.String(), Description: inc.Name,
			IncomeID:    sql.NullInt64{Int64: inc.ID, Valid: true},
			IncomeMonth: sql.NullString{String: c.Month.String(), Valid: true},
			CreatedBy:   userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("confirm income: %w", err)
		}
		return nil
	})
	if err != nil {
		return ledger.Entry{}, err
	}
	return s.ledger.Entry(ctx, entryID)
}

// occurrence returns the expected date of the income in month m, if it happens that month.
func occurrence(inc Income, m civil.Month) (civil.Date, bool) {
	if inc.Kind == KindEventual {
		return *inc.ExpectedOn, inc.ExpectedOn.MonthOf() == m
	}
	if monthIndex(m) < monthIndex(inc.StartMonth) {
		return civil.Date{}, false
	}
	on := m.Day(inc.Day)
	if inc.EndOn != nil && inc.EndOn.Before(on) {
		return civil.Date{}, false
	}
	return on, true
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }

// validate normalizes the input and checks the fields of its kind.
func validate(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Kind != KindRecurring && in.Kind != KindEventual:
		return in, ErrInvalidKind
	case in.Name == "":
		return in, ErrNameRequired
	case utf8.RuneCountInString(in.Name) > maxName:
		return in, ErrNameTooLong
	}
	if err := ledger.CheckPositive(in.AmountCents); err != nil {
		return in, err
	}
	if in.Kind == KindEventual {
		if in.ExpectedOn.IsZero() {
			return in, ErrExpectedRequired
		}
		in.Day, in.StartMonth, in.EndOn = 0, civil.Month{}, nil
		return in, nil
	}
	switch {
	case in.Day < 1 || in.Day > 31:
		return in, ErrInvalidDay
	case in.StartMonth == civil.Month{}:
		return in, ErrStartRequired
	case in.EndOn != nil && in.EndOn.Before(in.StartMonth.First()):
		return in, ErrEndBeforeStart
	}
	in.ExpectedOn = civil.Date{}
	return in, nil
}

// checkRefs ensures the person exists and the account accepts entries.
func checkRefs(ctx context.Context, q *store.Queries, in Input) error {
	if _, err := q.GetPerson(ctx, in.PersonID); errors.Is(err, sql.ErrNoRows) {
		return ErrPersonNotFound
	} else if err != nil {
		return err
	}
	return ledger.CheckActiveAccount(ctx, q, in.AccountID)
}

// toParams converts a validated input to its stored columns.
func toParams(in Input) store.UpdateIncomeParams {
	p := store.UpdateIncomeParams{Name: in.Name, PersonID: in.PersonID, AccountID: in.AccountID, AmountCents: in.AmountCents}
	if in.Kind == KindRecurring {
		p.Day = sql.NullInt64{Int64: int64(in.Day), Valid: true}
		p.StartMonth = sql.NullString{String: in.StartMonth.String(), Valid: true}
		if in.EndOn != nil {
			p.EndOn = sql.NullString{String: in.EndOn.String(), Valid: true}
		}
	} else {
		p.ExpectedOn = sql.NullString{String: in.ExpectedOn.String(), Valid: true}
	}
	return p
}

// fromRow converts a stored income, flagging end dates that are near or past (RF-12).
func fromRow(r store.ListIncomesRow, today civil.Date) (Income, error) {
	inc := Income{
		ID: r.ID, Kind: Kind(r.Kind), Name: r.Name, PersonID: r.PersonID, PersonName: r.PersonName,
		AccountID: r.AccountID, AccountName: r.AccountName, AmountCents: r.AmountCents, Day: int(r.Day.Int64),
	}
	if r.StartMonth.Valid {
		m, err := civil.ParseMonth(r.StartMonth.String)
		if err != nil {
			return Income{}, err
		}
		inc.StartMonth = m
	}
	for _, f := range []struct {
		src sql.NullString
		dst **civil.Date
	}{{r.EndOn, &inc.EndOn}, {r.ExpectedOn, &inc.ExpectedOn}} {
		if !f.src.Valid {
			continue
		}
		d, err := civil.ParseDate(f.src.String)
		if err != nil {
			return Income{}, err
		}
		*f.dst = &d
	}
	if inc.EndOn != nil {
		inc.Ended = inc.EndOn.Before(today)
		inc.EndingSoon = !inc.Ended && !today.AddDays(EndingSoonDays).Before(*inc.EndOn)
	}
	return inc, nil
}
