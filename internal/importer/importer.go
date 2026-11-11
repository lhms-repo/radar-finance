package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/problem"
)

// UncategorizedKey is the system key of Outros, Sem categoria definida, used for lines without a rule.
const UncategorizedKey = "uncategorized"

// maxLines caps the lines of one import, far above a real invoice.
const maxLines = 1_000

// Business errors returned by the service.
var (
	ErrRuleNotFound    = problem.NotFound("rule_not_found", "Regra de apelido não encontrada.")
	ErrRuleOriginal    = problem.Invalid("rule_original_required", "Informe o texto original do estabelecimento.")
	ErrRuleName        = problem.Invalid("rule_name_required", "Informe o nome amigável.")
	ErrRuleCategory    = problem.Invalid("rule_category_invalid", "Escolha uma subcategoria ativa.")
	ErrNothingToImport = problem.Invalid("import_nothing", "Nenhuma linha para importar.")
	ErrTooManyLines    = problem.Invalid("import_too_many", "O arquivo tem linhas demais.")
	ErrImportNotFound  = problem.NotFound("import_not_found", "Importação não encontrada.")
)

// Rule renames a merchant of the CSV and gives it a default category (RF-54).
type Rule struct {
	ID           int64
	Original     string
	FriendlyName string
	CategoryID   *int64
	CategoryName string
	CreatedAt    time.Time
}

// RuleInput creates or replaces a rule.
type RuleInput struct {
	Original     string
	FriendlyName string
	CategoryID   *int64
}

// Duplicate is what already exists in the Radar and looks like the line (RN-20).
type Duplicate struct {
	Description string
	Date        *civil.Date
	AmountCents int64
}

// PreviewLine is a CSV line with the rule applied and the duplicate check, before anything is saved (RF-53).
type PreviewLine struct {
	Line
	Index        int
	FriendlyName string
	CategoryID   *int64
	RuleID       *int64
	Duplicate    *Duplicate
	// Skip is the suggestion: payments and duplicates are not imported unless the user says so.
	Skip bool
}

// CommitLine is a line as confirmed by the user, with the name and category chosen.
type CommitLine struct {
	Line
	FriendlyName string
	CategoryID   *int64
	Skip         bool
	// Remember saves the name and category as a rule for the next imports (RF-55).
	Remember bool
}

// Result tells what an import did.
type Result struct {
	ImportID   int64
	Created    int
	Skipped    int
	RulesSaved int
}

// Import is one import of the history.
type Import struct {
	ID            int64
	CardID        int64
	CardName      string
	InvoiceMonth  civil.Month
	FileName      string
	LineCount     int
	PurchaseCount int
	CreatedByName string
	CreatedAt     time.Time
}

// Service reads, previews and records invoice imports.
type Service struct {
	conn  *sql.DB
	cards *card.Service
	now   func() time.Time
}

// NewService creates the import service.
func NewService(conn *sql.DB, cards *card.Service) *Service {
	return &Service{conn: conn, cards: cards, now: time.Now}
}

// Normalize returns the key a rule matches: lower case, single spaces (RN-19).
func Normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Rules lists the alias rules, newest first for each original text.
func (s *Service) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := store.New(s.conn).ListAliasRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := make([]Rule, 0, len(rows))
	for _, r := range rows {
		rule := Rule{ID: r.ID, Original: r.Original, FriendlyName: r.FriendlyName, CategoryName: r.CategoryName, CreatedAt: r.CreatedAt}
		if r.CategoryID.Valid {
			id := r.CategoryID.Int64
			rule.CategoryID = &id
		}
		out = append(out, rule)
	}
	return out, nil
}

// CreateRule adds a rule; a newer rule for the same text wins over the older ones (RF-54, RN-19).
func (s *Service) CreateRule(ctx context.Context, userID int64, in RuleInput) (Rule, error) {
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		original, name, err := checkRule(ctx, q, in)
		if err != nil {
			return err
		}
		r, err := q.CreateAliasRule(ctx, store.CreateAliasRuleParams{Original: original, FriendlyName: name, CategoryID: nullID(in.CategoryID), CreatedBy: userID, CreatedAt: s.now().UTC()})
		if err != nil {
			return fmt.Errorf("create rule: %w", err)
		}
		id = r.ID
		return nil
	})
	if err != nil {
		return Rule{}, err
	}
	return s.rule(ctx, id)
}

// UpdateRule replaces a rule.
func (s *Service) UpdateRule(ctx context.Context, id int64, in RuleInput) (Rule, error) {
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetAliasRule(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrRuleNotFound
		} else if err != nil {
			return err
		}
		original, name, err := checkRule(ctx, q, in)
		if err != nil {
			return err
		}
		return q.UpdateAliasRule(ctx, store.UpdateAliasRuleParams{Original: original, FriendlyName: name, CategoryID: nullID(in.CategoryID), ID: id})
	})
	if err != nil {
		return Rule{}, err
	}
	return s.rule(ctx, id)
}

// DeleteRule removes a rule.
func (s *Service) DeleteRule(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetAliasRule(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrRuleNotFound
		} else if err != nil {
			return err
		}
		return q.DeleteAliasRule(ctx, id)
	})
}

// Preview reads the CSV and returns each line with the rule applied and possible duplicates flagged,
// without saving anything (RF-53, RF-55, RF-56, RN-19, RN-20).
func (s *Service) Preview(ctx context.Context, cardID int64, invoice civil.Month, content string) ([]PreviewLine, error) {
	q := store.New(s.conn)
	if _, err := s.cards.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	lines, err := ParseNubank(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	if len(lines) > maxLines {
		return nil, ErrTooManyLines
	}
	rules, err := s.latestRules(ctx)
	if err != nil {
		return nil, err
	}
	fallback, err := q.GetCategoryBySystemKey(ctx, sql.NullString{String: UncategorizedKey, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("uncategorized category: %w", err)
	}
	recurring, err := s.cards.RecurrenceChargesIn(ctx, q, cardID, invoice)
	if err != nil {
		return nil, err
	}
	out := make([]PreviewLine, 0, len(lines))
	for i, l := range lines {
		p := PreviewLine{Line: l, Index: i, FriendlyName: l.Merchant}
		if r, ok := rules[Normalize(l.Merchant)]; ok {
			id := r.ID
			p.RuleID, p.FriendlyName, p.CategoryID = &id, r.FriendlyName, r.CategoryID
		}
		if p.CategoryID == nil && l.Kind == LinePurchase {
			id := fallback
			p.CategoryID = &id
		}
		if l.Kind == LinePayment {
			p.Skip = true
		} else if dup, err := s.duplicate(ctx, q, cardID, l, recurring); err != nil {
			return nil, err
		} else if dup != nil {
			p.Duplicate, p.Skip = dup, true
		}
		out = append(out, p)
	}
	return out, nil
}

// Commit records the confirmed lines on the chosen invoice and saves the rules asked for, in one transaction (RF-53, RF-55, RF-57).
func (s *Service) Commit(ctx context.Context, userID, cardID int64, invoice civil.Month, fileName string, lines []CommitLine) (Result, error) {
	if len(lines) == 0 {
		return Result{}, ErrNothingToImport
	}
	if len(lines) > maxLines {
		return Result{}, ErrTooManyLines
	}
	var res Result
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		now := s.now().UTC()
		imp, err := q.CreateImport(ctx, store.CreateImportParams{
			CardID: cardID, InvoiceMonth: invoice.String(), FileName: truncate(strings.TrimSpace(fileName), 200), LineCount: int64(len(lines)), CreatedBy: userID, CreatedAt: now,
		})
		if err != nil {
			if db.IsForeignKeyViolation(err) {
				return card.ErrCardNotFound
			}
			return fmt.Errorf("create import: %w", err)
		}
		res.ImportID = imp.ID
		rules, err := s.latestRulesTx(ctx, q)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if l.Skip || l.Kind == LinePayment {
				res.Skipped++
				continue
			}
			name := strings.TrimSpace(l.FriendlyName)
			if name == "" {
				name = l.Merchant
			}
			id, err := s.cards.ImportLine(ctx, q, userID, card.ImportedLine{
				CardID: cardID, Invoice: invoice, Date: l.Date, Merchant: name, CategoryID: l.CategoryID, AmountCents: l.AmountCents,
				Credit: l.Kind == LineCredit, Installment: l.Installment, Installments: l.Installments,
			})
			if err != nil {
				return err
			}
			if err := q.SetPurchaseImport(ctx, store.SetPurchaseImportParams{ImportID: sql.NullInt64{Int64: imp.ID, Valid: true}, ID: id}); err != nil {
				return err
			}
			res.Created++
			if !l.Remember {
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
		return Result{}, err
	}
	return res, nil
}

// Imports lists the import history, newest first.
func (s *Service) Imports(ctx context.Context) ([]Import, error) {
	rows, err := store.New(s.conn).ListImports(ctx)
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	out := make([]Import, 0, len(rows))
	for _, r := range rows {
		m, err := civil.ParseMonth(r.InvoiceMonth)
		if err != nil {
			return nil, err
		}
		out = append(out, Import{
			ID: r.ID, CardID: r.CardID, CardName: r.CardName, InvoiceMonth: m, FileName: r.FileName, LineCount: int(r.LineCount),
			PurchaseCount: int(r.PurchaseCount), CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// Undo removes every purchase of an import and the import itself, unless a share was already received.
func (s *Service) Undo(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetImport(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrImportNotFound
		} else if err != nil {
			return err
		}
		ids, err := q.ListImportPurchases(ctx, sql.NullInt64{Int64: id, Valid: true})
		if err != nil {
			return err
		}
		for _, pid := range ids {
			if err := card.CheckNoReceived(ctx, q, pid); err != nil {
				return err
			}
			if err := q.DeletePurchase(ctx, pid); err != nil {
				return err
			}
		}
		return q.DeleteImport(ctx, id)
	})
}

// duplicate looks for a card line with the same amount bought within a day, or a recurrence with the same amount on the invoice (RN-20).
func (s *Service) duplicate(ctx context.Context, q *store.Queries, cardID int64, l Line, recurring []card.Recurrence) (*Duplicate, error) {
	rows, err := q.CardItemsNear(ctx, store.CardItemsNearParams{CardID: cardID, FirstDay: l.Date.AddDays(-1).String(), LastDay: l.Date.AddDays(1).String()})
	if err != nil {
		return nil, fmt.Errorf("items near: %w", err)
	}
	want := l.AmountCents
	if l.Kind == LineCredit {
		want = -want
	}
	for _, r := range rows {
		if r.AmountCents == want {
			on, err := civil.ParseDate(r.PurchasedOn)
			if err != nil {
				return nil, err
			}
			return &Duplicate{Description: r.Merchant, Date: &on, AmountCents: r.AmountCents}, nil
		}
	}
	if l.Kind == LinePurchase {
		for _, r := range recurring {
			if r.AmountCents == l.AmountCents {
				return &Duplicate{Description: r.Description, AmountCents: r.AmountCents}, nil
			}
		}
	}
	return nil, nil
}

// latestRules maps each normalized original to its most recent rule (RN-19).
func (s *Service) latestRules(ctx context.Context) (map[string]Rule, error) {
	return s.latestRulesTx(ctx, store.New(s.conn))
}

// latestRulesTx maps each normalized original to its most recent rule with the given queries.
func (s *Service) latestRulesTx(ctx context.Context, q *store.Queries) (map[string]Rule, error) {
	rows, err := q.ListAliasRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := map[string]Rule{}
	for _, r := range rows {
		// Rows come newest first for each original, so the first one wins.
		if _, seen := out[r.Original]; seen {
			continue
		}
		rule := Rule{ID: r.ID, Original: r.Original, FriendlyName: r.FriendlyName}
		if r.CategoryID.Valid {
			id := r.CategoryID.Int64
			rule.CategoryID = &id
		}
		out[r.Original] = rule
	}
	return out, nil
}

// rule reads one rule.
func (s *Service) rule(ctx context.Context, id int64) (Rule, error) {
	all, err := s.Rules(ctx)
	if err != nil {
		return Rule{}, err
	}
	for _, r := range all {
		if r.ID == id {
			return r, nil
		}
	}
	return Rule{}, ErrRuleNotFound
}

// checkRule normalizes the original, trims the name and checks the category.
func checkRule(ctx context.Context, q *store.Queries, in RuleInput) (string, string, error) {
	original := Normalize(in.Original)
	name := strings.TrimSpace(in.FriendlyName)
	switch {
	case original == "" || utf8.RuneCountInString(original) > 120:
		return "", "", ErrRuleOriginal
	case name == "" || utf8.RuneCountInString(name) > 80:
		return "", "", ErrRuleName
	}
	if in.CategoryID != nil {
		c, err := q.GetSelectableCategory(ctx, *in.CategoryID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (!c.Active || !c.ParentActive)) {
			return "", "", ErrRuleCategory
		}
		if err != nil {
			return "", "", err
		}
	}
	return original, name, nil
}

// sameID compares two optional ids.
func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// nullID converts an optional id to its SQL form.
func nullID(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
