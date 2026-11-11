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

// errBadKind is returned for a line kind outside purchase, credit and payment.
var errBadKind = problem.Invalid("import_kind_invalid", "Tipo de linha inválido.")

// ListAliasRules lists the alias rules (RF-54).
func (a *API) ListAliasRules(ctx context.Context, _ gen.ListAliasRulesRequestObject) (gen.ListAliasRulesResponseObject, error) {
	rules, err := a.d.Imports.Rules(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListAliasRules200JSONResponse, 0, len(rules))
	for _, r := range rules {
		out = append(out, toRule(r))
	}
	return out, nil
}

// CreateAliasRule creates an alias rule (RF-54, RN-19).
func (a *API) CreateAliasRule(ctx context.Context, req gen.CreateAliasRuleRequestObject) (gen.CreateAliasRuleResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	r, err := a.d.Imports.CreateRule(ctx, auth.UserID(ctx), importer.RuleInput{Original: req.Body.Original, FriendlyName: req.Body.FriendlyName, CategoryID: req.Body.CategoryId})
	if err != nil {
		return nil, err
	}
	return gen.CreateAliasRule201JSONResponse(toRule(r)), nil
}

// UpdateAliasRule replaces an alias rule.
func (a *API) UpdateAliasRule(ctx context.Context, req gen.UpdateAliasRuleRequestObject) (gen.UpdateAliasRuleResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	r, err := a.d.Imports.UpdateRule(ctx, req.Id, importer.RuleInput{Original: req.Body.Original, FriendlyName: req.Body.FriendlyName, CategoryID: req.Body.CategoryId})
	if err != nil {
		return nil, err
	}
	return gen.UpdateAliasRule200JSONResponse(toRule(r)), nil
}

// DeleteAliasRule deletes an alias rule.
func (a *API) DeleteAliasRule(ctx context.Context, req gen.DeleteAliasRuleRequestObject) (gen.DeleteAliasRuleResponseObject, error) {
	if err := a.d.Imports.DeleteRule(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteAliasRule204Response{}, nil
}

// PreviewImport reads the CSV and returns the lines with rules and duplicates (RF-53, RF-56).
func (a *API) PreviewImport(ctx context.Context, req gen.PreviewImportRequestObject) (gen.PreviewImportResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Body.InvoiceMonth)
	if err != nil {
		return nil, errBadMonth
	}
	lines, err := a.d.Imports.Preview(ctx, req.Body.CardId, m, req.Body.Content)
	if err != nil {
		var le *importer.LineError
		if errors.As(err, &le) {
			return nil, problem.Invalid("import_line_invalid", "%s", le.Error())
		}
		return nil, err
	}
	out := make(gen.PreviewImport200JSONResponse, 0, len(lines))
	for _, l := range lines {
		p := gen.ImportPreviewLine{
			Index: l.Index, Date: apiDate(l.Date), Title: l.Title, Merchant: l.Merchant, AmountCents: l.AmountCents,
			Kind: gen.ImportPreviewLineKind(l.Kind), FriendlyName: l.FriendlyName, CategoryId: l.CategoryID, RuleId: l.RuleID, Skip: l.Skip,
		}
		if l.Installments > 0 {
			n, total := l.Installment, l.Installments
			p.Installment, p.Installments = &n, &total
		}
		if d := l.Duplicate; d != nil {
			p.Duplicate = &gen.ImportDuplicate{Description: d.Description, Date: apiDatePtr(d.Date), AmountCents: d.AmountCents}
		}
		out = append(out, p)
	}
	return out, nil
}

// CommitImport records the confirmed lines (RF-53, RF-55, RF-57).
func (a *API) CommitImport(ctx context.Context, req gen.CommitImportRequestObject) (gen.CommitImportResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := civil.ParseMonth(req.Body.InvoiceMonth)
	if err != nil {
		return nil, errBadMonth
	}
	lines := make([]importer.CommitLine, 0, len(req.Body.Lines))
	for _, l := range req.Body.Lines {
		kind := importer.LineKind(l.Kind)
		if kind != importer.LinePurchase && kind != importer.LineCredit && kind != importer.LinePayment {
			return nil, errBadKind
		}
		cl := importer.CommitLine{
			Line: importer.Line{
				Date: fromDate(l.Date), Title: l.Title, Merchant: l.Merchant, AmountCents: l.AmountCents, Kind: kind,
			},
			FriendlyName: l.FriendlyName, CategoryID: l.CategoryId, Skip: l.Skip, Remember: l.Remember, Split: fromSplitInput(l.Split),
		}
		if l.Installment != nil && l.Installments != nil {
			cl.Installment, cl.Installments = *l.Installment, *l.Installments
		}
		lines = append(lines, cl)
	}
	res, err := a.d.Imports.Commit(ctx, auth.UserID(ctx), req.Body.CardId, m, deref(req.Body.FileName), lines)
	if err != nil {
		return nil, err
	}
	return gen.CommitImport201JSONResponse{ImportId: res.ImportID, Created: res.Created, Skipped: res.Skipped, RulesSaved: res.RulesSaved}, nil
}

// ListImports returns the import history.
func (a *API) ListImports(ctx context.Context, _ gen.ListImportsRequestObject) (gen.ListImportsResponseObject, error) {
	list, err := a.d.Imports.Imports(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListImports200JSONResponse, 0, len(list))
	for _, i := range list {
		out = append(out, gen.ImportRecord{
			Id: i.ID, CardId: i.CardID, CardName: i.CardName, InvoiceMonth: i.InvoiceMonth.String(), FileName: i.FileName,
			LineCount: i.LineCount, PurchaseCount: i.PurchaseCount, CreatedByName: i.CreatedByName, CreatedAt: i.CreatedAt,
		})
	}
	return out, nil
}

// UndoImport removes the purchases of an import.
func (a *API) UndoImport(ctx context.Context, req gen.UndoImportRequestObject) (gen.UndoImportResponseObject, error) {
	if err := a.d.Imports.Undo(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.UndoImport204Response{}, nil
}

// toRule converts a rule to the API model.
func toRule(r importer.Rule) gen.AliasRule {
	return gen.AliasRule{Id: r.ID, Original: r.Original, FriendlyName: r.FriendlyName, CategoryId: r.CategoryID, CategoryName: r.CategoryName, CreatedAt: r.CreatedAt}
}
