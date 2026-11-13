// Package alelo manages the Alelo meal cards: the monthly credit confirmed by
// the user, free text purchases filed as groceries and the balance carried
// over the months (RF-31 to RF-34, RN-15).
package alelo

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
	"github.com/italogfx/radar-finance/internal/membersplit"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// GroceriesKey is the system key of Alimentação, Compras para casa (RN-15).
const GroceriesKey = "groceries"

// styles are the looks of the card drawing: the classic green and the Alelo Tudo identity.
var styles = map[string]bool{"classic": true, "tudo": true}

// maxName is the longest card name accepted, in characters.
const maxName = 40

// Business errors returned by the service.
var (
	ErrNotFound            = problem.NotFound("alelo_not_found", "Cartão Alelo não encontrado.")
	ErrNameRequired        = problem.Invalid("alelo_name_required", "Informe o nome do cartão Alelo.")
	ErrNameTooLong         = problem.Invalid("alelo_name_too_long", "Use no máximo %d caracteres no nome.", maxName)
	ErrDuplicateName       = problem.Conflict("alelo_duplicate_name", "Já existe um cartão Alelo com esse nome.")
	ErrInvalidDay          = problem.Invalid("alelo_day_invalid", "Informe um dia de crédito entre 1 e 31.")
	ErrInvalidBalance      = problem.Invalid("alelo_balance_invalid", "Informe um saldo de zero para cima.")
	ErrPersonNotFound      = problem.Invalid("alelo_person_not_found", "Escolha o titular.")
	ErrInactive            = problem.Conflict("alelo_inactive", "Este cartão Alelo está desativado.")
	ErrInUse               = problem.Conflict("alelo_in_use", "Este cartão Alelo já tem créditos ou compras. Desative-o para tirá-lo das opções.")
	ErrNoCredit            = problem.Invalid("alelo_no_credit", "Não há crédito previsto para este mês.")
	ErrAlreadyConfirmed    = problem.Conflict("alelo_credit_confirmed", "O crédito deste mês já foi confirmado.")
	ErrCreditNotFound      = problem.NotFound("alelo_credit_not_found", "Crédito não encontrado.")
	ErrDescriptionRequired = problem.Invalid("alelo_description_required", "Descreva a compra.")
	ErrPurchaseNotFound    = problem.NotFound("alelo_purchase_not_found", "Compra não encontrada.")
	ErrInvalidStyle        = problem.Invalid("alelo_style_invalid", "Escolha o visual do cartão.")
	ErrHolderUsedElsewhere = problem.Conflict("alelo_holder_used_elsewhere", "Este Alelo tem compras em outro grupo, então o titular não pode mudar.")
)

// Card is an Alelo card with its current balance.
type Card struct {
	ID                 int64
	Name               string
	PersonID           int64
	PersonName         string
	MonthlyCreditCents int64
	CreditDay          int
	BalanceCents       int64
	StartMonth         civil.Month
	Style              string
	Active             bool
}

// Input registers a card with its balance on the day (RF-31).
type Input struct {
	Name                string
	PersonID            int64
	MonthlyCreditCents  int64
	CreditDay           int
	InitialBalanceCents int64
	Style               string
}

// Changes edits a card; the starting balance stays as registered.
type Changes struct {
	Name               string
	PersonID           int64
	MonthlyCreditCents int64
	CreditDay          int
	Style              string
	Active             bool
}

// CreditState is the credit of a month: expected, and once confirmed, what came in (RF-32).
type CreditState struct {
	ExpectedCents int64
	ExpectedOn    civil.Date
	Confirmed     bool
	AmountCents   int64
	CreditedOn    *civil.Date
}

// PurchaseItem is one purchase of the month.
type PurchaseItem struct {
	ID            int64
	Date          civil.Date
	Description   string
	AmountCents   int64
	CreatedByName string
}

// MonthView is the month of a card: balance carried in, credit, purchases and balance carried out (RN-15).
type MonthView struct {
	Month        civil.Month
	OpeningCents int64
	Credit       CreditState
	Purchases    []PurchaseItem
	SpentCents   int64
	ClosingCents int64
	// HasCredit is false before the first month of the card.
	HasCredit bool
}

// Credit confirms the credit of a month; zero fields take the monthly value and today.
type Credit struct {
	AleloID     int64
	Month       civil.Month
	AmountCents int64
	Date        civil.Date
}

// Purchase is a purchase with a free description (RF-33).
type Purchase struct {
	AleloID     int64
	Date        civil.Date
	Description string
	AmountCents int64
	// Members divides the expense between the members of the group; nil keeps the weights (RN-28).
	Members *membersplit.Rule
}

// Service reads and writes Alelo cards.
type Service struct {
	conn  *sql.DB
	now   func() time.Time
	today func() civil.Date
}

// NewService creates the Alelo service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// List returns the cards of the active group's members with the current balance, active first (RF-82).
func (s *Service) List(ctx context.Context) ([]Card, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.conn).ListAleloCards(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list alelo cards: %w", err)
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		start, err := civil.ParseMonth(r.StartMonth)
		if err != nil {
			return nil, err
		}
		out = append(out, Card{
			ID: r.ID, Name: r.Name, PersonID: r.PersonID, PersonName: r.PersonName, MonthlyCreditCents: r.MonthlyCreditCents,
			CreditDay: int(r.CreditDay), BalanceCents: r.BalanceCents, StartMonth: start, Style: r.Style, Active: r.Active,
		})
	}
	return out, nil
}

// Get returns one card.
func (s *Service) Get(ctx context.Context, id int64) (Card, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Card{}, err
	}
	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}
	return Card{}, ErrNotFound
}

// Create registers a card; credits are planned from the current month (RF-31).
func (s *Service) Create(ctx context.Context, userID int64, in Input) (Card, error) {
	name, err := validate(in.Name, in.MonthlyCreditCents, in.CreditDay, in.Style)
	if err != nil {
		return Card{}, err
	}
	if in.InitialBalanceCents < 0 || in.InitialBalanceCents > ledger.MaxAmountCents {
		return Card{}, ErrInvalidBalance
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkRefs(ctx, q, name, in.PersonID, 0); err != nil {
			return err
		}
		now := s.now().UTC()
		c, err := q.CreateAleloCard(ctx, store.CreateAleloCardParams{
			Name: name, PersonID: in.PersonID, MonthlyCreditCents: in.MonthlyCreditCents, CreditDay: int64(in.CreditDay),
			InitialBalanceCents: in.InitialBalanceCents, StartMonth: s.today().MonthOf().String(), Style: in.Style, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create alelo card: %w", err)
		}
		id = c.ID
		return nil
	})
	if err != nil {
		return Card{}, err
	}
	return s.Get(ctx, id)
}

// Update edits a card.
func (s *Service) Update(ctx context.Context, id int64, c Changes) (Card, error) {
	name, err := validate(c.Name, c.MonthlyCreditCents, c.CreditDay, c.Style)
	if err != nil {
		return Card{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Card{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := getCard(ctx, q, id)
		if err != nil {
			return err
		}
		if err := checkRefs(ctx, q, name, c.PersonID, id); err != nil {
			return err
		}
		// A new holder outside another group would hide this card from it, with its purchases still counted there (RF-82).
		if c.PersonID != cur.PersonID {
			used, err := q.AleloUsedElsewhere(ctx, store.AleloUsedElsewhereParams{AleloID: id, GroupID: groupID})
			if err != nil {
				return err
			}
			if used != 0 {
				return ErrHolderUsedElsewhere
			}
		}
		return q.UpdateAleloCard(ctx, store.UpdateAleloCardParams{
			Name: name, PersonID: c.PersonID, MonthlyCreditCents: c.MonthlyCreditCents, CreditDay: int64(c.CreditDay),
			Style: c.Style, Active: c.Active, UpdatedAt: s.now().UTC(), ID: id, GroupID: groupID,
		})
	})
	if err != nil {
		return Card{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes a card without credits or purchases.
func (s *Service) Delete(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getCard(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteAleloCard(ctx, store.DeleteAleloCardParams{ID: id, GroupID: groupID}); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrInUse
			}
			return fmt.Errorf("delete alelo card: %w", err)
		}
		return nil
	})
}

// Month returns the month of a card: balance carried in, credit, purchases and balance carried out (RN-15).
func (s *Service) Month(ctx context.Context, id int64, m civil.Month) (MonthView, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return MonthView{}, err
	}
	q := store.New(s.conn)
	c, err := getCard(ctx, q, id)
	if err != nil {
		return MonthView{}, err
	}
	opening, err := q.AleloBalanceBefore(ctx, store.AleloBalanceBeforeParams{GroupID: groupID, Month: m.String(), FirstDay: m.First().String(), AleloID: id})
	if err != nil {
		return MonthView{}, fmt.Errorf("alelo opening balance: %w", err)
	}
	v := MonthView{
		Month: m, OpeningCents: opening, Purchases: []PurchaseItem{}, HasCredit: monthIndex(m) >= monthIndex(c.StartMonth),
		Credit: CreditState{ExpectedCents: c.MonthlyCreditCents, ExpectedOn: m.Day(c.CreditDay)},
	}
	cr, err := q.GetAleloCredit(ctx, store.GetAleloCreditParams{AleloID: id, Month: m.String()})
	switch {
	case err == nil:
		on, err := civil.ParseDate(cr.CreditedOn)
		if err != nil {
			return MonthView{}, err
		}
		v.Credit.Confirmed, v.Credit.AmountCents, v.Credit.CreditedOn = true, cr.AmountCents, &on
	case !errors.Is(err, sql.ErrNoRows):
		return MonthView{}, err
	}
	rows, err := q.ListAleloPurchases(ctx, store.ListAleloPurchasesParams{GroupID: groupID, AleloID: id, FirstDay: m.First().String(), LastDay: m.Last().String()})
	if err != nil {
		return MonthView{}, fmt.Errorf("list alelo purchases: %w", err)
	}
	for _, r := range rows {
		on, err := civil.ParseDate(r.PurchasedOn)
		if err != nil {
			return MonthView{}, err
		}
		v.Purchases = append(v.Purchases, PurchaseItem{ID: r.ID, Date: on, Description: r.Description, AmountCents: r.AmountCents, CreatedByName: r.CreatedByName})
		v.SpentCents += r.AmountCents
	}
	v.ClosingCents = v.OpeningCents + v.Credit.AmountCents - v.SpentCents
	return v, nil
}

// ConfirmCredit records the credit of a month when it falls; until then it is only expected (RF-32).
func (s *Service) ConfirmCredit(ctx context.Context, userID int64, cr Credit) (MonthView, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		c, err := getCard(ctx, q, cr.AleloID)
		if err != nil {
			return err
		}
		if monthIndex(cr.Month) < monthIndex(c.StartMonth) {
			return ErrNoCredit
		}
		if _, err := q.GetAleloCredit(ctx, store.GetAleloCreditParams{AleloID: c.ID, Month: cr.Month.String()}); err == nil {
			return ErrAlreadyConfirmed
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		amount, on := cr.AmountCents, cr.Date
		if amount == 0 {
			amount = c.MonthlyCreditCents
		}
		if on.IsZero() {
			on = s.today()
		}
		if err := ledger.CheckPositive(amount); err != nil {
			return err
		}
		return q.CreateAleloCredit(ctx, store.CreateAleloCreditParams{
			AleloID: c.ID, Month: cr.Month.String(), AmountCents: amount, CreditedOn: on.String(), CreatedBy: userID, CreatedAt: s.now().UTC(),
		})
	})
	if err != nil {
		return MonthView{}, err
	}
	return s.Month(ctx, cr.AleloID, cr.Month)
}

// UndoCredit removes the confirmation of a month; the credit is expected again.
func (s *Service) UndoCredit(ctx context.Context, id int64, m civil.Month) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getCard(ctx, q, id); err != nil {
			return err
		}
		n, err := q.DeleteAleloCredit(ctx, store.DeleteAleloCreditParams{AleloID: id, Month: m.String()})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrCreditNotFound
		}
		return nil
	})
}

// AddPurchase records a purchase with a free description, filed as Alimentação, Compras para casa (RF-33, RN-15).
func (s *Service) AddPurchase(ctx context.Context, userID int64, p Purchase) (PurchaseItem, error) {
	var out PurchaseItem
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		var err error
		out, err = s.AddPurchaseTx(ctx, q, userID, p)
		return err
	})
	return out, err
}

// AddPurchaseTx records the purchase inside a transaction of the caller.
func (s *Service) AddPurchaseTx(ctx context.Context, q *store.Queries, userID int64, p Purchase) (PurchaseItem, error) {
	desc := strings.TrimSpace(p.Description)
	if desc == "" {
		return PurchaseItem{}, ErrDescriptionRequired
	}
	if utf8.RuneCountInString(desc) > 120 {
		return PurchaseItem{}, ledger.ErrDescriptionTooLong
	}
	if err := ledger.CheckPositive(p.AmountCents); err != nil {
		return PurchaseItem{}, err
	}
	if p.Date.IsZero() {
		return PurchaseItem{}, ledger.ErrDateRequired
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return PurchaseItem{}, err
	}
	c, err := getCard(ctx, q, p.AleloID)
	if err != nil {
		return PurchaseItem{}, err
	}
	if !c.Active {
		return PurchaseItem{}, ErrInactive
	}
	groceries, err := q.GetCategoryBySystemKey(ctx, sql.NullString{String: GroceriesKey, Valid: true})
	if err != nil {
		return PurchaseItem{}, fmt.Errorf("groceries category: %w", err)
	}
	r, err := q.CreateAleloPurchase(ctx, store.CreateAleloPurchaseParams{
		AleloID: c.ID, PurchasedOn: p.Date.String(), Description: desc, AmountCents: p.AmountCents, CategoryID: groceries,
		CreatedBy: userID, CreatedAt: s.now().UTC(), GroupID: groupID,
	})
	if err != nil {
		return PurchaseItem{}, fmt.Errorf("create alelo purchase: %w", err)
	}
	if err := membersplit.Save(ctx, q, groupID, membersplit.KindAlelo, r.ID, p.Members, p.AmountCents); err != nil {
		return PurchaseItem{}, err
	}
	return PurchaseItem{ID: r.ID, Date: p.Date, Description: desc, AmountCents: r.AmountCents}, nil
}

// DeletePurchase removes a purchase.
func (s *Service) DeletePurchase(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetAleloPurchase(ctx, store.GetAleloPurchaseParams{ID: id, GroupID: groupID}); errors.Is(err, sql.ErrNoRows) {
			return ErrPurchaseNotFound
		} else if err != nil {
			return err
		}
		return q.DeleteAleloPurchase(ctx, store.DeleteAleloPurchaseParams{ID: id, GroupID: groupID})
	})
}

// validate trims the name and checks name, monthly credit and credit day.
func validate(name string, credit int64, day int, style string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrNameRequired
	case utf8.RuneCountInString(name) > maxName:
		return "", ErrNameTooLong
	case day < 1 || day > 31:
		return "", ErrInvalidDay
	case !styles[style]:
		return "", ErrInvalidStyle
	}
	return name, ledger.CheckPositive(credit)
}

// checkRefs ensures the holder is a member of the active group and the name is free.
func checkRefs(ctx context.Context, q *store.Queries, name string, personID, self int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	if _, err := q.GetGroupPerson(ctx, store.GetGroupPersonParams{ID: personID, GroupID: groupID}); errors.Is(err, sql.ErrNoRows) {
		return ErrPersonNotFound
	} else if err != nil {
		return err
	}
	rows, err := q.ListAleloCards(ctx, groupID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != self && strings.EqualFold(r.Name, name) {
			return ErrDuplicateName
		}
	}
	return nil
}

// getCard reads one card.
func getCard(ctx context.Context, q *store.Queries, id int64) (Card, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return Card{}, err
	}
	r, err := q.GetAleloCard(ctx, store.GetAleloCardParams{ID: id, GroupID: groupID})
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	start, err := civil.ParseMonth(r.StartMonth)
	if err != nil {
		return Card{}, err
	}
	return Card{
		ID: r.ID, Name: r.Name, PersonID: r.PersonID, MonthlyCreditCents: r.MonthlyCreditCents, CreditDay: int(r.CreditDay),
		StartMonth: start, Style: r.Style, Active: r.Active,
	}, nil
}

// monthIndex orders months as a single number.
func monthIndex(m civil.Month) int { return m.Year*12 + int(m.Month) }
