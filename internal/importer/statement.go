package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/scope"
)

// maxStatementName bounds the description an imported line gets, the limit of an entry.
const maxStatementName = 120

// StatementPreviewLine is a statement line with what it becomes, before anything is saved (RF-86 to RF-89, RN-32).
type StatementPreviewLine struct {
	StatementLine
	Index        int
	FriendlyName string
	CategoryID   *int64
	RuleID       *int64
	Duplicate    *Duplicate
	// IncomeID, IncomeName and IncomeMonth are the expected income the salary confirms (RF-88).
	IncomeID    *int64
	IncomeName  string
	IncomeMonth civil.Month
	// CardID and InvoiceMonth are the invoice the card payment pays, suggested by its total (RF-89).
	CardID       *int64
	InvoiceMonth civil.Month
	// Skip is the suggestion: duplicates and card payments without an invoice that matches are not imported.
	Skip bool
}

// StatementPreview is the whole statement as it would be imported, with its closing balance (RF-90).
type StatementPreview struct {
	Lines        []StatementPreviewLine
	ClosingCents int64
}

// StatementCommitLine is a statement line as confirmed by the user.
type StatementCommitLine struct {
	StatementLine
	FriendlyName string
	CategoryID   *int64
	Skip         bool
	// Remember saves the name and category as a rule for the next imports (RF-55).
	Remember     bool
	IncomeID     *int64
	IncomeMonth  civil.Month
	CardID       *int64
	InvoiceMonth civil.Month
}

// StatementResult tells what an import of a statement did, with the balances to compare (RF-90).
type StatementResult struct {
	ImportID     int64
	Created      int
	Skipped      int
	RulesSaved   int
	ClosingCents int64
}

// StatementImport is one statement import of the history.
type StatementImport struct {
	ID            int64
	AccountID     int64
	AccountName   string
	FileName      string
	LineCount     int
	EntryCount    int
	ClosingCents  int64
	CreatedByName string
	CreatedAt     time.Time
}

// Statements previews, records and undoes imports of account statements (RF-86 to RF-90).
type Statements struct {
	conn    *sql.DB
	ledger  *ledger.Service
	incomes *income.Service
	cards   *card.Service
	now     func() time.Time
}

// NewStatements creates the statement import service.
func NewStatements(conn *sql.DB, cards *card.Service) *Statements {
	return &Statements{conn: conn, ledger: ledger.NewService(conn), incomes: income.NewService(conn), cards: cards, now: time.Now}
}

// Preview reads the statement and returns each line with what it becomes, without saving anything (RF-86 to RF-89, RN-32).
func (s *Statements) Preview(ctx context.Context, accountID int64, content string) (StatementPreview, error) {
	userID, err := actor(ctx)
	if err != nil {
		return StatementPreview{}, err
	}
	if _, err := s.ledger.GetAccount(ctx, accountID); err != nil {
		return StatementPreview{}, err
	}
	st, err := ParseSantander(strings.NewReader(content))
	if err != nil {
		return StatementPreview{}, err
	}
	if len(st.Lines) > maxLines {
		return StatementPreview{}, ErrTooManyLines
	}
	q := store.New(s.conn)
	rules, err := latestRules(ctx, q, userID)
	if err != nil {
		return StatementPreview{}, err
	}
	defaults, err := defaultCategories(ctx, q)
	if err != nil {
		return StatementPreview{}, err
	}
	expected, err := s.expectedIncomes(ctx, accountID, st.Lines)
	if err != nil {
		return StatementPreview{}, err
	}
	invoices, err := s.closedInvoices(ctx, st.Lines)
	if err != nil {
		return StatementPreview{}, err
	}
	out := StatementPreview{ClosingCents: st.ClosingCents, Lines: make([]StatementPreviewLine, 0, len(st.Lines))}
	used := map[int64]bool{}
	for i, l := range st.Lines {
		p := StatementPreviewLine{StatementLine: l, Index: i, FriendlyName: truncate(l.Merchant, maxStatementName)}
		if l.Kind != StatementCardPayment {
			if r, ok := rules[Normalize(l.Merchant)]; ok {
				id := r.ID
				p.RuleID, p.FriendlyName, p.CategoryID = &id, r.FriendlyName, r.CategoryID
			}
		}
		if p.CategoryID == nil && l.Kind == StatementExpense {
			id := defaults[l.CategoryKey]
			p.CategoryID = &id
		}
		dup, err := statementDuplicate(ctx, q, accountID, l, used)
		if err != nil {
			return StatementPreview{}, err
		}
		if dup != nil {
			p.Duplicate, p.Skip = dup, true
		}
		switch l.Kind {
		case StatementSalary:
			if o, ok := expected.take(l); ok {
				id := o.IncomeID
				p.IncomeID, p.IncomeName, p.IncomeMonth = &id, o.Name, o.ExpectedOn.MonthOf()
			}
		case StatementCardPayment:
			if inv, ok := invoices.take(l); ok {
				id := inv.CardID
				p.CardID, p.InvoiceMonth = &id, inv.Month
			} else {
				p.Skip = true
			}
		}
		out.Lines = append(out.Lines, p)
	}
	return out, nil
}

// Commit records the confirmed lines in the account and saves the rules asked for, in one transaction. A card
// payment is recorded in the personal space of the card owner, where the invoice is paid (RF-83, RF-86 to RF-90).
func (s *Statements) Commit(ctx context.Context, userID, accountID int64, fileName string, closingCents int64, lines []StatementCommitLine) (StatementResult, error) {
	if len(lines) == 0 {
		return StatementResult{}, ErrNothingToImport
	}
	if len(lines) > maxLines {
		return StatementResult{}, ErrTooManyLines
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return StatementResult{}, err
	}
	if _, err := s.ledger.GetAccount(ctx, accountID); err != nil {
		return StatementResult{}, err
	}
	res := StatementResult{ClosingCents: closingCents}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		now := s.now().UTC()
		imp, err := q.CreateStatementImport(ctx, store.CreateStatementImportParams{
			GroupID: groupID, AccountID: accountID, FileName: truncate(strings.TrimSpace(fileName), 200), LineCount: int64(len(lines)),
			ClosingCents: closingCents, CreatedBy: userID, CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create statement import: %w", err)
		}
		res.ImportID = imp.ID
		rules, err := latestRules(ctx, q, userID)
		if err != nil {
			return err
		}
		defaults, err := defaultCategories(ctx, q)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if l.Skip || (l.Kind == StatementCardPayment && l.CardID == nil) {
				res.Skipped++
				continue
			}
			name := truncate(strings.TrimSpace(l.FriendlyName), maxStatementName)
			if name == "" {
				name = truncate(l.Merchant, maxStatementName)
			}
			id, err := s.record(ctx, q, userID, accountID, l, name, defaults)
			if err != nil {
				return err
			}
			if err := q.SetEntryStatementImport(ctx, store.SetEntryStatementImportParams{ImportID: sql.NullInt64{Int64: imp.ID, Valid: true}, ID: id}); err != nil {
				return err
			}
			res.Created++
			if !l.Remember || l.Kind == StatementCardPayment {
				continue
			}
			key := Normalize(l.Merchant)
			if r, ok := rules[key]; ok && r.FriendlyName == name && sameID(r.CategoryID, l.CategoryID) {
				continue
			}
			if _, _, err := checkRule(ctx, q, RuleInput{Original: l.Merchant, FriendlyName: name, CategoryID: l.CategoryID}); err != nil {
				return err
			}
			r, err := q.CreateAliasRule(ctx, store.CreateAliasRuleParams{Original: key, FriendlyName: name, CategoryID: nullID(l.CategoryID), CreatedBy: userID, CreatedAt: now})
			if err != nil {
				return fmt.Errorf("save rule: %w", err)
			}
			rules[key] = Rule{ID: r.ID, FriendlyName: name, CategoryID: l.CategoryID}
			res.RulesSaved++
		}
		return nil
	})
	if err != nil {
		return StatementResult{}, err
	}
	return res, nil
}

// record writes one confirmed line as what it is and returns the entry id (RF-87 to RF-89).
func (s *Statements) record(ctx context.Context, q *store.Queries, userID, accountID int64, l StatementCommitLine, name string, defaults map[string]int64) (int64, error) {
	switch l.Kind {
	case StatementExpense:
		category := defaults[l.CategoryKey]
		if l.CategoryID != nil {
			category = *l.CategoryID
		}
		return s.ledger.AddExpenseTx(ctx, q, userID, ledger.NewExpense{AccountID: accountID, AmountCents: l.AmountCents, Date: l.Date, CategoryID: category, Description: name})
	case StatementSalary:
		if l.IncomeID != nil {
			return s.incomes.ConfirmTx(ctx, q, userID, income.Confirmation{IncomeID: *l.IncomeID, Month: l.IncomeMonth, AmountCents: l.AmountCents, Date: l.Date, AccountID: accountID})
		}
		return s.ledger.AddIncomeTx(ctx, q, userID, ledger.NewIncome{AccountID: accountID, AmountCents: l.AmountCents, Date: l.Date, Description: name})
	case StatementCardPayment:
		personal, err := s.ownerSpace(ctx, q, *l.CardID)
		if err != nil {
			return 0, err
		}
		return s.cards.PayTx(personal, q, userID, card.Payment{CardID: *l.CardID, Month: l.InvoiceMonth, AccountID: accountID, Date: l.Date, AmountCents: l.AmountCents})
	default:
		return s.ledger.AddIncomeTx(ctx, q, userID, ledger.NewIncome{AccountID: accountID, AmountCents: l.AmountCents, Date: l.Date, Description: name})
	}
}

// ownerSpace returns the context of the personal space of the card owner when she is the user importing, where
// the invoice is paid; any other user keeps the active space, and the payment there is refused (RF-83).
func (s *Statements) ownerSpace(ctx context.Context, q *store.Queries, cardID int64) (context.Context, error) {
	c, err := s.cards.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	viewer, err := ledger.ViewerPerson(ctx, q)
	if err != nil {
		return nil, err
	}
	if viewer == 0 || viewer != c.PersonID {
		return ctx, nil
	}
	personal, err := q.PersonalGroupOf(ctx, sql.NullInt64{Int64: viewer, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, nil
	}
	if err != nil {
		return nil, err
	}
	return scope.WithGroup(ctx, personal), nil
}

// Imports lists the statement imports of the current user in the active group, newest first.
func (s *Statements) Imports(ctx context.Context) ([]StatementImport, error) {
	userID, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.conn).ListStatementImports(ctx, store.ListStatementImportsParams{GroupID: groupID, CreatedBy: userID})
	if err != nil {
		return nil, fmt.Errorf("list statement imports: %w", err)
	}
	out := make([]StatementImport, 0, len(rows))
	for _, r := range rows {
		out = append(out, StatementImport{
			ID: r.ID, AccountID: r.AccountID, AccountName: r.AccountName, FileName: r.FileName, LineCount: int(r.LineCount),
			EntryCount: int(r.EntryCount), ClosingCents: r.ClosingCents, CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// Undo removes every entry of a statement import and the import itself; invoice payments and income
// confirmations go with their entries (RF-90).
func (s *Statements) Undo(ctx context.Context, id int64) error {
	userID, err := actor(ctx)
	if err != nil {
		return err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetStatementImport(ctx, store.GetStatementImportParams{ID: id, GroupID: groupID, CreatedBy: userID}); errors.Is(err, sql.ErrNoRows) {
			return ErrImportNotFound
		} else if err != nil {
			return err
		}
		if err := q.DeleteStatementImportEntries(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
			return err
		}
		return q.DeleteStatementImport(ctx, store.DeleteStatementImportParams{ID: id, GroupID: groupID, CreatedBy: userID})
	})
}

// statementDuplicate looks for an entry of the account, in any space, with the same signed amount within a day of
// the line; what matched one line is not used again (RN-32).
func statementDuplicate(ctx context.Context, q *store.Queries, accountID int64, l StatementLine, used map[int64]bool) (*Duplicate, error) {
	signed := l.AmountCents
	if l.Kind == StatementExpense || l.Kind == StatementCardPayment {
		signed = -signed
	}
	rows, err := q.AccountEntriesNear(ctx, store.AccountEntriesNearParams{
		AccountID: accountID, AmountCents: signed, FirstDay: l.Date.AddDays(-1).String(), LastDay: l.Date.AddDays(1).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("entries near: %w", err)
	}
	for _, r := range rows {
		if used[r.ID] {
			continue
		}
		on, err := civil.ParseDate(r.OccurredOn)
		if err != nil {
			return nil, err
		}
		used[r.ID] = true
		return &Duplicate{Description: r.Description, Date: &on, AmountCents: r.AmountCents}, nil
	}
	return nil, nil
}

// expectedPool holds the expected incomes of the account not yet confirmed, each one usable by a single salary line.
type expectedPool struct{ list []income.Occurrence }

// take returns the expected income with the amount of the line in the month of the line or, failing that, in the
// next one, since a salary may fall a few days early (RF-88).
func (p *expectedPool) take(l StatementLine) (income.Occurrence, bool) {
	for _, m := range []civil.Month{l.Date.MonthOf(), l.Date.MonthOf().Add(1)} {
		for i, o := range p.list {
			if o.ExpectedCents == l.AmountCents && o.ExpectedOn.MonthOf() == m {
				p.list = append(p.list[:i], p.list[i+1:]...)
				return o, true
			}
		}
	}
	return income.Occurrence{}, false
}

// expectedIncomes lists the recurring incomes of the account still expected in the months of the salary lines.
func (s *Statements) expectedIncomes(ctx context.Context, accountID int64, lines []StatementLine) (*expectedPool, error) {
	months := map[civil.Month]bool{}
	for _, l := range lines {
		if l.Kind == StatementSalary {
			months[l.Date.MonthOf()], months[l.Date.MonthOf().Add(1)] = true, true
		}
	}
	pool := &expectedPool{}
	for m := range months {
		v, err := s.incomes.Month(ctx, m)
		if err != nil {
			return nil, err
		}
		for _, o := range v.Occurrences {
			if !o.Confirmed && o.AccountID == accountID && o.Kind == income.KindRecurring {
				pool.list = append(pool.list, o)
			}
		}
	}
	return pool, nil
}

// invoiceChoice is a closed and unpaid invoice a card payment line may pay.
type invoiceChoice struct {
	CardID     int64
	Month      civil.Month
	TotalCents int64
}

// invoicePool holds the invoices a payment may pay, each one usable by a single line.
type invoicePool struct{ list []invoiceChoice }

// take returns the invoice closed before the payment day whose total is the amount of the line (RF-89).
func (p *invoicePool) take(l StatementLine) (invoiceChoice, bool) {
	for i, inv := range p.list {
		if inv.TotalCents == l.AmountCents && inv.Month.First().Before(l.Date) {
			p.list = append(p.list[:i], p.list[i+1:]...)
			return inv, true
		}
	}
	return invoiceChoice{}, false
}

// closedInvoices lists, for each card the user owns, the invoice closed at the date of each payment line and not
// paid yet, read in her personal space where it is paid (RF-83, RF-89).
func (s *Statements) closedInvoices(ctx context.Context, lines []StatementLine) (*invoicePool, error) {
	pool := &invoicePool{}
	var payments []StatementLine
	for _, l := range lines {
		if l.Kind == StatementCardPayment {
			payments = append(payments, l)
		}
	}
	if len(payments) == 0 {
		return pool, nil
	}
	cards, err := s.cards.ListCards(ctx)
	if err != nil {
		return nil, err
	}
	q := store.New(s.conn)
	seen := map[invoiceChoice]bool{}
	for _, c := range cards {
		if !c.Owned {
			continue
		}
		personal, err := s.ownerSpace(ctx, q, c.ID)
		if err != nil {
			return nil, err
		}
		for _, l := range payments {
			m := c.Cycle().InvoiceFor(l.Date).Add(-1)
			key := invoiceChoice{CardID: c.ID, Month: m}
			if seen[key] {
				continue
			}
			seen[key] = true
			inv, err := s.cards.Invoice(personal, c.ID, m)
			if err != nil {
				return nil, err
			}
			if inv.Full && inv.Payment == nil && inv.TotalCents > 0 {
				pool.list = append(pool.list, invoiceChoice{CardID: c.ID, Month: m, TotalCents: inv.TotalCents})
			}
		}
	}
	return pool, nil
}

// defaultCategories maps the category key of a statement line to its subcategory; the empty key is Sem categoria
// definida, which also stands in for a key whose subcategory is gone (RF-87).
func defaultCategories(ctx context.Context, q *store.Queries) (map[string]int64, error) {
	fallback, err := q.GetCategoryBySystemKey(ctx, sql.NullString{String: UncategorizedKey, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("uncategorized category: %w", err)
	}
	out := map[string]int64{"": fallback}
	for _, key := range []string{KeyBankFees, KeyTaxes, KeyInterest, KeyInvestments} {
		id, err := q.GetCategoryBySystemKey(ctx, sql.NullString{String: key, Valid: true})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			out[key] = fallback
		case err != nil:
			return nil, err
		default:
			out[key] = id
		}
	}
	return out, nil
}
