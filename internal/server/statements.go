package server

import (
	"context"
	"errors"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/importer"
	"github.com/italogfx/radar-finance/internal/problem"
)

// DetectImport tells which known CSV a file is (RF-86).
func (a *API) DetectImport(_ context.Context, req gen.DetectImportRequestObject) (gen.DetectImportResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	f, err := importer.Detect(req.Body.Content)
	if err != nil {
		return nil, err
	}
	return gen.DetectImport200JSONResponse{Format: gen.ImportDetectResultFormat(f)}, nil
}

// PreviewStatement reads the Santander statement and returns what each line becomes (RF-86 to RF-89, RN-32).
func (a *API) PreviewStatement(ctx context.Context, req gen.PreviewStatementRequestObject) (gen.PreviewStatementResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Statements.Preview(ctx, req.Body.AccountId, req.Body.Content)
	if err != nil {
		var le *importer.LineError
		if errors.As(err, &le) {
			return nil, problem.Invalid("import_line_invalid", "%s", le.Error())
		}
		return nil, err
	}
	out := gen.StatementPreview{ClosingCents: p.ClosingCents, Lines: make([]gen.StatementPreviewLine, 0, len(p.Lines))}
	for _, l := range p.Lines {
		line := gen.StatementPreviewLine{
			Index: l.Index, Date: apiDate(l.Date), Description: l.Description, Detail: l.Detail, Merchant: l.Merchant, AmountCents: l.AmountCents,
			Kind: gen.StatementKind(l.Kind), CategoryKey: l.CategoryKey, FriendlyName: l.FriendlyName, CategoryId: l.CategoryID, RuleId: l.RuleID,
			IncomeId: l.IncomeID, IncomeName: optional(l.IncomeName), IncomeMonth: monthPtr(l.IncomeMonth), CardId: l.CardID,
			InvoiceMonth: monthPtr(l.InvoiceMonth), Skip: l.Skip,
		}
		if d := l.Duplicate; d != nil {
			line.Duplicate = &gen.ImportDuplicate{Description: d.Description, Date: apiDatePtr(d.Date), AmountCents: d.AmountCents}
		}
		out.Lines = append(out.Lines, line)
	}
	return gen.PreviewStatement200JSONResponse(out), nil
}

// CommitStatement records the confirmed lines of a statement and tells the real balance after it to the holder (RF-86 to RF-90).
func (a *API) CommitStatement(ctx context.Context, req gen.CommitStatementRequestObject) (gen.CommitStatementResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	lines := make([]importer.StatementCommitLine, 0, len(req.Body.Lines))
	for _, l := range req.Body.Lines {
		kind := importer.StatementKind(l.Kind)
		switch kind {
		case importer.StatementExpense, importer.StatementIncome, importer.StatementSalary, importer.StatementCardPayment:
		default:
			return nil, errBadKind
		}
		cl := importer.StatementCommitLine{
			StatementLine: importer.StatementLine{
				Date: fromDate(l.Date), Description: l.Description, Detail: l.Detail, Merchant: l.Merchant, AmountCents: l.AmountCents,
				Kind: kind, CategoryKey: l.CategoryKey,
			},
			FriendlyName: l.FriendlyName, CategoryID: l.CategoryId, Skip: l.Skip, Remember: l.Remember, IncomeID: l.IncomeId, CardID: l.CardId,
		}
		var err error
		if cl.IncomeMonth, err = statementMonth(l.IncomeMonth); err != nil {
			return nil, err
		}
		if cl.InvoiceMonth, err = statementMonth(l.InvoiceMonth); err != nil {
			return nil, err
		}
		lines = append(lines, cl)
	}
	res, err := a.d.Statements.Commit(ctx, auth.UserID(ctx), req.Body.AccountId, deref(req.Body.FileName), req.Body.ClosingCents, lines)
	if err != nil {
		return nil, err
	}
	out := gen.StatementResult{ImportId: res.ImportID, Created: res.Created, Skipped: res.Skipped, RulesSaved: res.RulesSaved, ClosingCents: res.ClosingCents}
	// The real balance tells every space, so only the holder reads it to compare with the statement (RF-83, RF-90).
	if acc, err := a.d.Ledger.GetAccount(ctx, req.Body.AccountId); err == nil && acc.Owned {
		balance := acc.BalanceCents
		out.BalanceCents = &balance
	}
	return gen.CommitStatement201JSONResponse(out), nil
}

// ListStatementImports returns the statement import history.
func (a *API) ListStatementImports(ctx context.Context, _ gen.ListStatementImportsRequestObject) (gen.ListStatementImportsResponseObject, error) {
	list, err := a.d.Statements.Imports(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListStatementImports200JSONResponse, 0, len(list))
	for _, i := range list {
		out = append(out, gen.StatementImportRecord{
			Id: i.ID, AccountId: i.AccountID, AccountName: i.AccountName, FileName: i.FileName, LineCount: i.LineCount,
			EntryCount: i.EntryCount, ClosingCents: i.ClosingCents, CreatedByName: i.CreatedByName, CreatedAt: i.CreatedAt,
		})
	}
	return out, nil
}

// UndoStatementImport removes the entries of a statement import (RF-90).
func (a *API) UndoStatementImport(ctx context.Context, req gen.UndoStatementImportRequestObject) (gen.UndoStatementImportResponseObject, error) {
	if err := a.d.Statements.Undo(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.UndoStatementImport204Response{}, nil
}

// monthPtr returns the month as text, or nil for the zero month.
func monthPtr(m civil.Month) *string {
	if m == (civil.Month{}) {
		return nil
	}
	s := m.String()
	return &s
}

// statementMonth parses an optional month of a statement line, the zero month when absent.
func statementMonth(s *string) (civil.Month, error) {
	if s == nil || *s == "" {
		return civil.Month{}, nil
	}
	m, err := civil.ParseMonth(*s)
	if err != nil {
		return civil.Month{}, errBadMonth
	}
	return m, nil
}
