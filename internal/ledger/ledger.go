// Package ledger manages bank accounts and the entries that move their money:
// balances, adjustments, transfers and one off expenses (RF-04 to RF-08, RN-08).
package ledger

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
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// MaxAmountCents caps any single amount at one billion reais, far above real use.
const MaxAmountCents int64 = 100_000_000_000

// Limits of free text fields, in characters.
const (
	maxAccountName = 40
	maxDescription = 120
)

// Kind classifies an entry. Only income and expense count as money earned or spent (RN-08).
type Kind string

// Entry kinds of this phase; later phases add their own.
const (
	KindExpense    Kind = "expense"
	KindIncome     Kind = "income"
	KindAdjustment Kind = "adjustment"
	KindTransfer   Kind = "transfer"
)

// savingsDepositKind mirrors savings.KindDepositEntry, which this package cannot import.
const savingsDepositKind = "savings_deposit"

// Business errors returned by the service.
var (
	ErrAccountNotFound     = problem.NotFound("account_not_found", "Conta não encontrada.")
	ErrAccountNameRequired = problem.Invalid("account_name_required", "Informe o nome da conta.")
	ErrAccountNameTooLong  = problem.Invalid("account_name_too_long", "Use no máximo %d caracteres no nome da conta.", maxAccountName)
	ErrDuplicateAccount    = problem.Conflict("account_duplicate_name", "Já existe uma conta com esse nome.")
	ErrPersonNotFound      = problem.Invalid("person_not_found", "Escolha o titular da conta.")
	ErrAccountInactive     = problem.Conflict("account_inactive", "Esta conta está desativada. Reative-a para lançar nela.")
	ErrAccountInUse        = problem.Conflict("account_in_use", "Esta conta já tem lançamentos e não pode ser excluída. Desative-a para tirá-la das opções.")
	ErrAmountRequired      = problem.Invalid("amount_required", "Informe um valor maior que zero.")
	ErrAmountTooLarge      = problem.Invalid("amount_too_large", "Valor alto demais. Confira os dígitos.")
	ErrDateRequired        = problem.Invalid("date_required", "Informe a data.")
	ErrDescriptionTooLong  = problem.Invalid("description_too_long", "Use no máximo %d caracteres na descrição.", maxDescription)
	ErrSameAccount         = problem.Invalid("transfer_same_account", "Escolha contas diferentes para a transferência.")
	ErrNoChange            = problem.Invalid("adjustment_no_change", "O saldo da conta já é esse valor.")
	ErrEntryNotFound       = problem.NotFound("entry_not_found", "Lançamento não encontrado.")
	ErrSavingsNegative     = problem.Conflict("savings_deposit_withdrawn", "Este aporte já foi retirado da caixinha. Apague a retirada antes, senão a caixinha fica com saldo negativo.")
)

// Account is a bank account with its current balance (RF-05).
type Account struct {
	ID                  int64
	Name                string
	PersonID            int64
	PersonName          string
	InitialBalanceCents int64
	BalanceCents        int64
	Active              bool
}

// NewAccount is the input to register an account (RF-04).
type NewAccount struct {
	Name                string
	PersonID            int64
	InitialBalanceCents int64
}

// AccountChanges holds optional edits; nil fields stay unchanged.
type AccountChanges struct {
	Name                *string
	PersonID            *int64
	InitialBalanceCents *int64
	Active              *bool
}

// Entry is one movement of an account, with names resolved for display.
type Entry struct {
	ID                     int64
	AccountID              int64
	Kind                   Kind
	AmountCents            int64
	Date                   civil.Date
	Confirmed              bool
	CategoryID             *int64
	CategoryName           string
	ParentCategoryName     string
	CategoryIcon           string
	Description            string
	TransferID             *int64
	CounterpartAccountName string
	CreatedByName          string
	CreatedAt              time.Time
}

// NewExpense is a one off expense paid straight from an account (section 5.6).
type NewExpense struct {
	AccountID   int64
	AmountCents int64
	Date        civil.Date
	CategoryID  int64
	Description string
}

// Adjustment sets the account balance to the value the bank shows (RF-06).
type Adjustment struct {
	AccountID    int64
	BalanceCents int64
	Date         civil.Date
	Description  string
}

// NewTransfer moves money between two accounts of the couple (RF-07).
type NewTransfer struct {
	FromAccountID int64
	ToAccountID   int64
	AmountCents   int64
	Date          civil.Date
	Description   string
}

// TransferResult identifies a transfer and its two entries.
type TransferResult struct {
	ID         int64
	OutEntryID int64
	InEntryID  int64
}

// Statement is the month of one account with its balances at the start and end.
type Statement struct {
	Month        civil.Month
	OpeningCents int64
	ClosingCents int64
	Entries      []Entry
}

// Flow sums the confirmed income and spending of a month, leaving out transfers and adjustments (RN-08).
type Flow struct {
	IncomeCents  int64
	ExpenseCents int64
}

// Service reads and writes accounts and entries.
type Service struct {
	conn *sql.DB
	now  func() time.Time
}

// NewService creates the ledger service.
func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn, now: time.Now}
}

// ListAccounts returns every account with its balance, active ones first.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := store.New(s.conn).ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, Account{
			ID: r.ID, Name: r.Name, PersonID: r.PersonID, PersonName: r.PersonName,
			InitialBalanceCents: r.InitialBalanceCents, BalanceCents: r.BalanceCents, Active: r.Active,
		})
	}
	return out, nil
}

// GetAccount returns one account with its balance.
func (s *Service) GetAccount(ctx context.Context, id int64) (Account, error) {
	all, err := s.ListAccounts(ctx)
	if err != nil {
		return Account{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrAccountNotFound
}

// CreateAccount registers an account with holder and initial balance (RF-04).
func (s *Service) CreateAccount(ctx context.Context, userID int64, in NewAccount) (Account, error) {
	name, err := cleanAccountName(in.Name)
	if err != nil {
		return Account{}, err
	}
	if err := checkAmount(in.InitialBalanceCents); err != nil {
		return Account{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := CheckPerson(ctx, q, in.PersonID); err != nil {
			return err
		}
		if err := checkUniqueAccount(ctx, q, name, 0); err != nil {
			return err
		}
		now := s.now().UTC()
		a, err := q.CreateAccount(ctx, store.CreateAccountParams{
			Name: name, PersonID: in.PersonID, InitialBalanceCents: in.InitialBalanceCents,
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create account: %w", err)
		}
		id = a.ID
		return nil
	})
	if err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, id)
}

// UpdateAccount edits name, holder, initial balance or active state; history stays.
func (s *Service) UpdateAccount(ctx context.Context, id int64, c AccountChanges) (Account, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		cur, err := getAccount(ctx, q, id)
		if err != nil {
			return err
		}
		next := store.UpdateAccountParams{
			Name: cur.Name, PersonID: cur.PersonID, InitialBalanceCents: cur.InitialBalanceCents,
			Active: cur.Active, UpdatedAt: s.now().UTC(), ID: id,
		}
		if c.Name != nil {
			if next.Name, err = cleanAccountName(*c.Name); err != nil {
				return err
			}
			if err := checkUniqueAccount(ctx, q, next.Name, id); err != nil {
				return err
			}
		}
		if c.PersonID != nil {
			if err := CheckPerson(ctx, q, *c.PersonID); err != nil {
				return err
			}
			next.PersonID = *c.PersonID
		}
		if c.InitialBalanceCents != nil {
			if err := checkAmount(*c.InitialBalanceCents); err != nil {
				return err
			}
			next.InitialBalanceCents = *c.InitialBalanceCents
		}
		if c.Active != nil {
			next.Active = *c.Active
		}
		return q.UpdateAccount(ctx, next)
	})
	if err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, id)
}

// DeleteAccount removes an account without entries; one with history must be deactivated.
func (s *Service) DeleteAccount(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getAccount(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteAccount(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrAccountInUse
			}
			return fmt.Errorf("delete account: %w", err)
		}
		return nil
	})
}

// AddExpense records a confirmed one off expense from an account, classified by a subcategory.
func (s *Service) AddExpense(ctx context.Context, userID int64, in NewExpense) (Entry, error) {
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		var err error
		id, err = s.AddExpenseTx(ctx, q, userID, in)
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	return s.Entry(ctx, id)
}

// AddExpenseTx records the expense inside a transaction of the caller and returns the entry id.
func (s *Service) AddExpenseTx(ctx context.Context, q *store.Queries, userID int64, in NewExpense) (int64, error) {
	if err := CheckPositive(in.AmountCents); err != nil {
		return 0, err
	}
	desc, err := CleanDescription(in.Description)
	if err != nil {
		return 0, err
	}
	if in.Date.IsZero() {
		return 0, ErrDateRequired
	}
	if err := CheckActiveAccount(ctx, q, in.AccountID); err != nil {
		return 0, err
	}
	if err := category.CheckSelectable(ctx, q, in.CategoryID); err != nil {
		return 0, err
	}
	e, err := s.insert(ctx, q, userID, store.CreateEntryParams{
		AccountID: in.AccountID, Kind: string(KindExpense), AmountCents: -in.AmountCents,
		OccurredOn: in.Date.String(), Confirmed: true,
		CategoryID: sql.NullInt64{Int64: in.CategoryID, Valid: true}, Description: desc,
	})
	return e.ID, err
}

// NewIncome is a one off income received in an account, outside the registered incomes (RF-13).
type NewIncome struct {
	AccountID   int64
	AmountCents int64
	Date        civil.Date
	Description string
}

// AddIncome records a confirmed one off income in an account (RF-13).
func (s *Service) AddIncome(ctx context.Context, userID int64, in NewIncome) (Entry, error) {
	if err := CheckPositive(in.AmountCents); err != nil {
		return Entry{}, err
	}
	desc, err := CleanDescription(in.Description)
	if err != nil {
		return Entry{}, err
	}
	if in.Date.IsZero() {
		return Entry{}, ErrDateRequired
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := CheckActiveAccount(ctx, q, in.AccountID); err != nil {
			return err
		}
		e, err := s.insert(ctx, q, userID, store.CreateEntryParams{
			AccountID: in.AccountID, Kind: string(KindIncome), AmountCents: in.AmountCents,
			OccurredOn: in.Date.String(), Confirmed: true, Description: desc,
		})
		id = e.ID
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	return s.Entry(ctx, id)
}

// AdjustBalance records the difference between the informed and the recorded balance (RF-06).
func (s *Service) AdjustBalance(ctx context.Context, userID int64, in Adjustment) (Entry, error) {
	if err := checkAmount(in.BalanceCents); err != nil {
		return Entry{}, err
	}
	desc, err := CleanDescription(in.Description)
	if err != nil {
		return Entry{}, err
	}
	if in.Date.IsZero() {
		return Entry{}, ErrDateRequired
	}
	var id int64
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := CheckActiveAccount(ctx, q, in.AccountID); err != nil {
			return err
		}
		current, err := q.AccountBalance(ctx, in.AccountID)
		if err != nil {
			return fmt.Errorf("read balance: %w", err)
		}
		diff := in.BalanceCents - current
		if diff == 0 {
			return ErrNoChange
		}
		e, err := s.insert(ctx, q, userID, store.CreateEntryParams{
			AccountID: in.AccountID, Kind: string(KindAdjustment), AmountCents: diff,
			OccurredOn: in.Date.String(), Confirmed: true, Description: desc,
		})
		id = e.ID
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	return s.Entry(ctx, id)
}

// Transfer moves money between two active accounts as a pair of linked entries (RF-07, RN-08).
func (s *Service) Transfer(ctx context.Context, userID int64, in NewTransfer) (TransferResult, error) {
	if in.FromAccountID == in.ToAccountID {
		return TransferResult{}, ErrSameAccount
	}
	if err := CheckPositive(in.AmountCents); err != nil {
		return TransferResult{}, err
	}
	desc, err := CleanDescription(in.Description)
	if err != nil {
		return TransferResult{}, err
	}
	if in.Date.IsZero() {
		return TransferResult{}, ErrDateRequired
	}
	var res TransferResult
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		for _, id := range []int64{in.FromAccountID, in.ToAccountID} {
			if err := CheckActiveAccount(ctx, q, id); err != nil {
				return err
			}
		}
		t, err := q.CreateTransfer(ctx, store.CreateTransferParams{
			FromAccountID: in.FromAccountID, ToAccountID: in.ToAccountID, AmountCents: in.AmountCents,
			OccurredOn: in.Date.String(), CreatedBy: userID, CreatedAt: s.now().UTC(),
		})
		if err != nil {
			return fmt.Errorf("create transfer: %w", err)
		}
		res.ID = t.ID
		link := sql.NullInt64{Int64: t.ID, Valid: true}
		out, err := s.insert(ctx, q, userID, store.CreateEntryParams{
			AccountID: in.FromAccountID, Kind: string(KindTransfer), AmountCents: -in.AmountCents,
			OccurredOn: in.Date.String(), Confirmed: true, Description: desc, TransferID: link,
		})
		if err != nil {
			return err
		}
		res.OutEntryID = out.ID
		inEntry, err := s.insert(ctx, q, userID, store.CreateEntryParams{
			AccountID: in.ToAccountID, Kind: string(KindTransfer), AmountCents: in.AmountCents,
			OccurredOn: in.Date.String(), Confirmed: true, Description: desc, TransferID: link,
		})
		res.InEntryID = inEntry.ID
		return err
	})
	if err != nil {
		return TransferResult{}, err
	}
	return res, nil
}

// DeleteEntry removes an entry; deleting either side of a transfer removes both.
func (s *Service) DeleteEntry(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		e, err := q.GetEntry(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEntryNotFound
		}
		if err != nil {
			return err
		}
		if e.TransferID.Valid {
			// entries.transfer_id cascades, so both sides go with the transfer.
			return q.DeleteTransfer(ctx, e.TransferID.Int64)
		}
		if e.Kind == savingsDepositKind {
			// The savings move cascades with the entry, so a deposit already withdrawn would leave the box below zero.
			balance, err := q.SavingsBalanceWithoutEntry(ctx, id)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && balance < 0 {
				return ErrSavingsNegative
			}
		}
		return q.DeleteEntry(ctx, id)
	})
}

// Statement lists the entries of one account in the month, newest first, with balances (RF-05).
func (s *Service) Statement(ctx context.Context, accountID int64, m civil.Month) (Statement, error) {
	q := store.New(s.conn)
	if _, err := getAccount(ctx, q, accountID); err != nil {
		return Statement{}, err
	}
	opening, err := q.AccountBalanceBefore(ctx, store.AccountBalanceBeforeParams{Day: m.First().String(), AccountID: accountID})
	if err != nil {
		return Statement{}, fmt.Errorf("opening balance: %w", err)
	}
	entries, err := s.list(ctx, q, accountID, m)
	if err != nil {
		return Statement{}, err
	}
	closing := opening
	for _, e := range entries {
		if e.Confirmed {
			closing += e.AmountCents
		}
	}
	return Statement{Month: m, OpeningCents: opening, ClosingCents: closing, Entries: entries}, nil
}

// MonthFlow sums confirmed income and spending of all accounts in the month (RN-08).
func (s *Service) MonthFlow(ctx context.Context, m civil.Month) (Flow, error) {
	r, err := store.New(s.conn).MonthFlow(ctx, store.MonthFlowParams{FirstDay: m.First().String(), LastDay: m.Last().String()})
	if err != nil {
		return Flow{}, fmt.Errorf("month flow: %w", err)
	}
	return Flow{IncomeCents: r.IncomeCents, ExpenseCents: r.ExpenseCents}, nil
}

// insert writes an entry stamped with the author and the current time (RF-03).
func (s *Service) insert(ctx context.Context, q *store.Queries, userID int64, p store.CreateEntryParams) (store.Entry, error) {
	now := s.now().UTC()
	p.CreatedBy, p.CreatedAt, p.UpdatedAt = userID, now, now
	e, err := q.CreateEntry(ctx, p)
	if err != nil {
		return store.Entry{}, fmt.Errorf("create entry: %w", err)
	}
	return e, nil
}

// Entry reads one entry with the display names.
func (s *Service) Entry(ctx context.Context, id int64) (Entry, error) {
	q := store.New(s.conn)
	e, err := q.GetEntry(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrEntryNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	d, err := civil.ParseDate(e.OccurredOn)
	if err != nil {
		return Entry{}, err
	}
	list, err := s.list(ctx, q, e.AccountID, d.MonthOf())
	if err != nil {
		return Entry{}, err
	}
	for _, x := range list {
		if x.ID == id {
			return x, nil
		}
	}
	return Entry{}, ErrEntryNotFound
}

// list reads the entries of an account in a month.
func (s *Service) list(ctx context.Context, q *store.Queries, accountID int64, m civil.Month) ([]Entry, error) {
	rows, err := q.ListAccountEntries(ctx, store.ListAccountEntriesParams{
		AccountID: accountID, FirstDay: m.First().String(), LastDay: m.Last().String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		d, err := civil.ParseDate(r.OccurredOn)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{
			ID: r.ID, AccountID: r.AccountID, Kind: Kind(r.Kind), AmountCents: r.AmountCents, Date: d,
			Confirmed: r.Confirmed, CategoryID: idPtr(r.CategoryID), CategoryName: r.CategoryName,
			ParentCategoryName: r.ParentCategoryName, CategoryIcon: r.CategoryIcon, Description: r.Description,
			TransferID: idPtr(r.TransferID), CounterpartAccountName: r.CounterpartAccountName,
			CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// getAccount reads an account row or returns ErrAccountNotFound.
func getAccount(ctx context.Context, q *store.Queries, id int64) (store.Account, error) {
	a, err := q.GetAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, ErrAccountNotFound
	}
	if err != nil {
		return store.Account{}, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// CheckActiveAccount ensures the account exists and accepts new entries.
func CheckActiveAccount(ctx context.Context, q *store.Queries, id int64) error {
	a, err := getAccount(ctx, q, id)
	if err != nil {
		return err
	}
	if !a.Active {
		return ErrAccountInactive
	}
	return nil
}

// CheckPerson ensures the holder is one of the couple.
func CheckPerson(ctx context.Context, q *store.Queries, id int64) error {
	if _, err := q.GetPerson(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return ErrPersonNotFound
	} else if err != nil {
		return fmt.Errorf("get person: %w", err)
	}
	return nil
}

// checkUniqueAccount rejects a name already used by another account, ignoring case.
func checkUniqueAccount(ctx context.Context, q *store.Queries, name string, self int64) error {
	rows, err := q.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != self && strings.EqualFold(r.Name, name) {
			return ErrDuplicateAccount
		}
	}
	return nil
}

// cleanAccountName trims and validates an account name.
func cleanAccountName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrAccountNameRequired
	}
	if utf8.RuneCountInString(name) > maxAccountName {
		return "", ErrAccountNameTooLong
	}
	return name, nil
}

// CleanDescription trims and validates a free description.
func CleanDescription(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if utf8.RuneCountInString(d) > maxDescription {
		return "", ErrDescriptionTooLong
	}
	return d, nil
}

// checkAmount bounds a signed amount such as a balance.
func checkAmount(cents int64) error {
	if cents > MaxAmountCents || cents < -MaxAmountCents {
		return ErrAmountTooLarge
	}
	return nil
}

// CheckPositive requires an amount above zero and within the bound.
func CheckPositive(cents int64) error {
	if cents <= 0 {
		return ErrAmountRequired
	}
	return checkAmount(cents)
}

// idPtr converts a nullable id to a pointer.
func idPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
