// Package card manages credit cards, their purchases, installments,
// recurrences and invoices (RF-14 to RF-23, RN-01 to RN-03, RN-07, RN-17).
package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
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

// Limits of the card fields.
const (
	maxCardName     = 40
	maxMerchant     = 80
	maxInstallments = 48
)

// Colors and finishes of the card drawing (section 5.4); graphite with metal is the main card look.
var (
	colors   = map[string]bool{"graphite": true, "purple": true, "blue": true, "green": true, "red": true, "orange": true, "gold": true, "silver": true}
	finishes = map[string]bool{"metal": true, "matte": true}
)

// digitsPattern accepts the last four digits of the card number.
var digitsPattern = regexp.MustCompile(`^[0-9]{4}$`)

// Business errors returned by the service.
var (
	ErrCardNotFound        = problem.NotFound("card_not_found", "Cartão não encontrado.")
	ErrNameRequired        = problem.Invalid("card_name_required", "Informe o nome do cartão.")
	ErrNameTooLong         = problem.Invalid("card_name_too_long", "Use no máximo %d caracteres no nome do cartão.", maxCardName)
	ErrDuplicateName       = problem.Conflict("card_duplicate_name", "Já existe um cartão com esse nome.")
	ErrInvalidLimit        = problem.Invalid("card_limit_invalid", "Informe um limite válido.")
	ErrInvalidDay          = problem.Invalid("card_day_invalid", "Os dias de fechamento e vencimento vão de 1 a 31.")
	ErrInvalidDigits       = problem.Invalid("card_digits_invalid", "Informe os quatro últimos dígitos do cartão.")
	ErrInvalidLook         = problem.Invalid("card_look_invalid", "Cor ou acabamento inválido.")
	ErrPersonNotFound      = problem.Invalid("card_person_not_found", "Escolha uma pessoa do casal.")
	ErrCardInactive        = problem.Conflict("card_inactive", "Este cartão está desativado. Reative-o para lançar nele.")
	ErrCardInUse           = problem.Conflict("card_in_use", "Este cartão já tem lançamentos e não pode ser excluído. Desative-o para tirá-lo das opções.")
	ErrPurchaseNotFound    = problem.NotFound("purchase_not_found", "Compra não encontrada.")
	ErrMerchantRequired    = problem.Invalid("purchase_merchant_required", "Informe o estabelecimento.")
	ErrMerchantTooLong     = problem.Invalid("purchase_merchant_too_long", "Use no máximo %d caracteres no estabelecimento.", maxMerchant)
	ErrInvalidInstallments = problem.Invalid("purchase_installments_invalid", "Informe de 1 a %d parcelas, com pelo menos um centavo em cada.", maxInstallments)
	ErrRecurrenceNotFound  = problem.NotFound("recurrence_not_found", "Recorrência não encontrada.")
	ErrDescriptionRequired = problem.Invalid("recurrence_description_required", "Informe a descrição da recorrência.")
	ErrEndBeforeStart      = problem.Invalid("recurrence_end_before_start", "A última fatura é anterior à primeira.")
	ErrRecurrenceInUse     = problem.Conflict("recurrence_in_use", "Esta recorrência já está em faturas pagas. Informe a última fatura para encerrá-la.")
	ErrInvoiceOpen         = problem.Conflict("invoice_open", "A fatura ainda está aberta. O pagamento fica disponível a partir do fechamento.")
	ErrNothingToPay        = problem.Conflict("invoice_nothing_to_pay", "Esta fatura não tem valor a pagar.")
	ErrAlreadyPaid         = problem.Conflict("invoice_already_paid", "Esta fatura já foi paga.")
)

// Card is a credit card with its limit use (RF-14, RF-20).
type Card struct {
	ID             int64
	Name           string
	PersonID       int64
	PersonName     string
	LimitCents     int64
	ClosingDay     int
	DueDay         int
	LastDigits     string
	Color          string
	Finish         string
	Active         bool
	LimitUsedCents int64
	LimitFreeCents int64
	// CurrentInvoice is the invoice a purchase made today would fall into.
	CurrentInvoice civil.Month
}

// Cycle returns the closing and due days of the card.
func (c Card) Cycle() Cycle { return Cycle{ClosingDay: c.ClosingDay, DueDay: c.DueDay} }

// CardInput holds every editable field of a card.
type CardInput struct {
	Name       string
	PersonID   int64
	LimitCents int64
	ClosingDay int
	DueDay     int
	LastDigits string
	Color      string
	Finish     string
}

// Service reads and writes cards, purchases, recurrences and invoices.
type Service struct {
	conn  *sql.DB
	now   func() time.Time
	today func() civil.Date
}

// NewService creates the card service; today is read in America/Sao_Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, now: time.Now, today: func() civil.Date { return civil.Today(loc) }}
}

// ListCards returns every card with its limit use, active ones first.
func (s *Service) ListCards(ctx context.Context) ([]Card, error) {
	q := store.New(s.conn)
	rows, err := q.ListCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		c := Card{
			ID: r.ID, Name: r.Name, PersonID: r.PersonID, PersonName: r.PersonName, LimitCents: r.LimitCents,
			ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay), LastDigits: r.LastDigits.String,
			Color: r.Color, Finish: r.Finish, Active: r.Active,
		}
		c.CurrentInvoice = c.Cycle().InvoiceFor(s.today())
		used, err := s.limitUsed(ctx, q, c)
		if err != nil {
			return nil, err
		}
		c.LimitUsedCents, c.LimitFreeCents = used, c.LimitCents-used
		out = append(out, c)
	}
	return out, nil
}

// GetCard returns one card with its limit use.
func (s *Service) GetCard(ctx context.Context, id int64) (Card, error) {
	all, err := s.ListCards(ctx)
	if err != nil {
		return Card{}, err
	}
	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}
	return Card{}, ErrCardNotFound
}

// CreateCard registers a card (RF-14).
func (s *Service) CreateCard(ctx context.Context, userID int64, in CardInput) (Card, error) {
	in, err := validateCard(in)
	if err != nil {
		return Card{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkCardRefs(ctx, q, in, 0); err != nil {
			return err
		}
		now := s.now().UTC()
		c, err := q.CreateCard(ctx, store.CreateCardParams{
			Name: in.Name, PersonID: in.PersonID, LimitCents: in.LimitCents, ClosingDay: int64(in.ClosingDay), DueDay: int64(in.DueDay),
			LastDigits: nullString(in.LastDigits), Color: in.Color, Finish: in.Finish, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create card: %w", err)
		}
		id = c.ID
		return nil
	})
	if err != nil {
		return Card{}, err
	}
	return s.GetCard(ctx, id)
}

// UpdateCard replaces the fields of a card; changing the closing day only affects purchases registered after it.
func (s *Service) UpdateCard(ctx context.Context, id int64, in CardInput, active bool) (Card, error) {
	in, err := validateCard(in)
	if err != nil {
		return Card{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getCard(ctx, q, id); err != nil {
			return err
		}
		if err := checkCardRefs(ctx, q, in, id); err != nil {
			return err
		}
		return q.UpdateCard(ctx, store.UpdateCardParams{
			Name: in.Name, PersonID: in.PersonID, LimitCents: in.LimitCents, ClosingDay: int64(in.ClosingDay), DueDay: int64(in.DueDay),
			LastDigits: nullString(in.LastDigits), Color: in.Color, Finish: in.Finish, Active: active, UpdatedAt: s.now().UTC(), ID: id,
		})
	})
	if err != nil {
		return Card{}, err
	}
	return s.GetCard(ctx, id)
}

// DeleteCard removes a card without purchases, recurrences or payments.
func (s *Service) DeleteCard(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getCard(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteCard(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrCardInUse
			}
			return fmt.Errorf("delete card: %w", err)
		}
		return nil
	})
}

// validateCard trims and checks the card fields.
func validateCard(in CardInput) (CardInput, error) {
	in.Name, in.LastDigits = strings.TrimSpace(in.Name), strings.TrimSpace(in.LastDigits)
	switch {
	case in.Name == "":
		return in, ErrNameRequired
	case utf8.RuneCountInString(in.Name) > maxCardName:
		return in, ErrNameTooLong
	case in.LimitCents < 0 || in.LimitCents > ledger.MaxAmountCents:
		return in, ErrInvalidLimit
	case in.ClosingDay < 1 || in.ClosingDay > 31 || in.DueDay < 1 || in.DueDay > 31:
		return in, ErrInvalidDay
	case in.LastDigits != "" && !digitsPattern.MatchString(in.LastDigits):
		return in, ErrInvalidDigits
	case !colors[in.Color] || !finishes[in.Finish]:
		return in, ErrInvalidLook
	}
	return in, nil
}

// checkCardRefs ensures the holder exists and the name is free.
func checkCardRefs(ctx context.Context, q *store.Queries, in CardInput, self int64) error {
	if err := checkPerson(ctx, q, in.PersonID); err != nil {
		return err
	}
	rows, err := q.ListCards(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != self && strings.EqualFold(r.Name, in.Name) {
			return ErrDuplicateName
		}
	}
	return nil
}

// checkPerson ensures the person is one of the couple.
func checkPerson(ctx context.Context, q *store.Queries, id int64) error {
	if _, err := q.GetPerson(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return ErrPersonNotFound
	} else if err != nil {
		return err
	}
	return nil
}

// getCard reads a card row or returns ErrCardNotFound.
func getCard(ctx context.Context, q *store.Queries, id int64) (Card, error) {
	r, err := q.GetCard(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrCardNotFound
	}
	if err != nil {
		return Card{}, fmt.Errorf("get card: %w", err)
	}
	return Card{
		ID: r.ID, Name: r.Name, PersonID: r.PersonID, LimitCents: r.LimitCents, ClosingDay: int(r.ClosingDay), DueDay: int(r.DueDay),
		LastDigits: r.LastDigits.String, Color: r.Color, Finish: r.Finish, Active: r.Active,
	}, nil
}

// activeCard reads a card that accepts new purchases.
func activeCard(ctx context.Context, q *store.Queries, id int64) (Card, error) {
	c, err := getCard(ctx, q, id)
	if err != nil {
		return Card{}, err
	}
	if !c.Active {
		return Card{}, ErrCardInactive
	}
	return c, nil
}

// nullString stores an empty string as NULL.
func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// nullID converts an optional id to its SQL form.
func nullID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

// idPtr converts a nullable id to a pointer.
func idPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

// WithToday replaces the clock of the service, so other packages can test date dependent results.
func (s *Service) WithToday(today func() civil.Date) *Service {
	s.today = today
	return s
}
