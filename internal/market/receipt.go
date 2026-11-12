package market

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/nfce"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// Errors of the receipt reading.
var (
	ErrReceiptImported = problem.Conflict("receipt_imported", "Essa nota fiscal já foi lida.")
	ErrReceiptNotFound = problem.NotFound("receipt_not_found", "Nota fiscal não encontrada.")
	ErrReceiptLine     = problem.Invalid("receipt_line_invalid", "Escolha um produto ou ignore cada linha da nota uma única vez.")
	ErrReceiptEmpty    = problem.Invalid("receipt_empty", "Escolha ao menos uma linha da nota.")
	ErrReceiptKey      = problem.Invalid("receipt_key_invalid", "A nota não trouxe a chave de acesso ou a data. Leia o QR code de novo.")
)

// cacheTTL keeps a downloaded receipt between the preview and the confirmation.
const cacheTTL = 15 * time.Minute

// Fetcher downloads a receipt from the address of its QR code.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (nfce.Receipt, error)
}

// receiptCache keeps recent downloads by address so the portal is asked once.
type receiptCache struct {
	mu    sync.Mutex
	items map[string]cachedReceipt
}

// cachedReceipt is a download and its time.
type cachedReceipt struct {
	receipt nfce.Receipt
	at      time.Time
}

// newReceiptCache returns an empty cache.
func newReceiptCache() *receiptCache {
	return &receiptCache{items: map[string]cachedReceipt{}}
}

// ReceiptPreview is a receipt with its market, when known, and each line with the learned link (RF-68, RF-69).
type ReceiptPreview struct {
	URL          string
	AccessKey    string
	StoreName    string
	StoreCNPJ    string
	Neighborhood string
	IssuedOn     civil.Date
	TotalCents   int64
	MarketID     *int64
	ImportedID   *int64
	Items        []PreviewItem
}

// PreviewItem is one receipt line with the product it is linked to, or ignored.
type PreviewItem struct {
	nfce.Item
	Index     int
	ProductID *int64
	Ignored   bool
}

// ReceiptCommit confirms a receipt: the market, existing or new, and the decision for each chosen line.
type ReceiptCommit struct {
	URL       string
	MarketID  int64
	NewMarket *MarketInput
	Items     []CommitItem
}

// CommitItem links a line to a product, existing or new, or ignores it from now on.
type CommitItem struct {
	Index      int
	ProductID  int64
	NewProduct *ProductInput
	Ignore     bool
}

// ReceiptResult tells what a confirmation recorded.
type ReceiptResult struct {
	ReceiptID int64
	MarketID  int64
	Prices    int
	Ignored   int
}

// ReceiptRecord is a receipt already read, for the history.
type ReceiptRecord struct {
	ID            int64
	MarketName    string
	IssuedOn      civil.Date
	TotalCents    int64
	ItemCount     int64
	PriceCount    int64
	CreatedByName string
}

// PreviewReceipt downloads the receipt and shows each line with what was learned for the market;
// without a market, the one with the receipt CNPJ is used (RF-68, RF-69).
func (s *Service) PreviewReceipt(ctx context.Context, url string, marketID int64) (ReceiptPreview, error) {
	rec, err := s.receipt(ctx, url)
	if err != nil {
		return ReceiptPreview{}, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return ReceiptPreview{}, err
	}
	q := store.New(s.conn)
	out := ReceiptPreview{
		URL: url, AccessKey: rec.AccessKey, StoreName: rec.StoreName, StoreCNPJ: rec.StoreCNPJ, Neighborhood: rec.Neighborhood,
		IssuedOn: rec.IssuedOn, TotalCents: rec.TotalCents, Items: make([]PreviewItem, 0, len(rec.Items)),
	}
	if id, err := q.ReceiptByKey(ctx, store.ReceiptByKeyParams{GroupID: groupID, AccessKey: rec.AccessKey}); err == nil {
		out.ImportedID = &id
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ReceiptPreview{}, err
	}
	if marketID == 0 && rec.StoreCNPJ != "" {
		if m, err := q.MarketByCNPJ(ctx, store.MarketByCNPJParams{GroupID: groupID, Cnpj: sql.NullString{String: rec.StoreCNPJ, Valid: true}}); err == nil {
			marketID = m.ID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return ReceiptPreview{}, err
		}
	}
	links := map[string]store.MarketLinksRow{}
	if marketID != 0 {
		if _, err := getMarket(ctx, q, marketID); err != nil {
			return ReceiptPreview{}, err
		}
		out.MarketID = &marketID
		rows, err := q.MarketLinks(ctx, marketID)
		if err != nil {
			return ReceiptPreview{}, fmt.Errorf("market links: %w", err)
		}
		for _, r := range rows {
			links[r.ItemKey] = r
		}
	}
	for i, it := range rec.Items {
		p := PreviewItem{Item: it, Index: i}
		if l, ok := links[itemKey(it)]; ok {
			p.Ignored = l.Ignored
			if l.ProductID.Valid {
				id := l.ProductID.Int64
				p.ProductID = &id
			}
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}

// CommitReceipt records the receipt, its prices with the receipt date and the links learned for the market (RF-68, RF-69).
func (s *Service) CommitReceipt(ctx context.Context, userID int64, in ReceiptCommit) (ReceiptResult, error) {
	rec, err := s.receipt(ctx, in.URL)
	if err != nil {
		return ReceiptResult{}, err
	}
	if len(rec.AccessKey) != 44 || rec.IssuedOn.IsZero() {
		return ReceiptResult{}, ErrReceiptKey
	}
	if len(in.Items) == 0 {
		return ReceiptResult{}, ErrReceiptEmpty
	}
	seen := map[int]bool{}
	for _, it := range in.Items {
		decisions := 0
		for _, set := range []bool{it.ProductID != 0, it.NewProduct != nil, it.Ignore} {
			if set {
				decisions++
			}
		}
		if it.Index < 0 || it.Index >= len(rec.Items) || seen[it.Index] || decisions != 1 {
			return ReceiptResult{}, ErrReceiptLine
		}
		seen[it.Index] = true
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return ReceiptResult{}, err
	}
	var res ReceiptResult
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.ReceiptByKey(ctx, store.ReceiptByKeyParams{GroupID: groupID, AccessKey: rec.AccessKey}); err == nil {
			return ErrReceiptImported
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		marketID, err := s.receiptMarket(ctx, q, userID, in, rec.StoreCNPJ)
		if err != nil {
			return err
		}
		receiptID, err := q.CreateReceipt(ctx, store.CreateReceiptParams{
			MarketID: marketID, AccessKey: rec.AccessKey, IssuedOn: rec.IssuedOn.String(), TotalCents: rec.TotalCents,
			ItemCount: int64(len(rec.Items)), CreatedBy: userID, CreatedAt: s.now().UTC(),
		})
		if err != nil {
			return fmt.Errorf("create receipt: %w", err)
		}
		res = ReceiptResult{ReceiptID: receiptID, MarketID: marketID}
		for _, it := range in.Items {
			line := rec.Items[it.Index]
			key := itemKey(line)
			if it.Ignore {
				if err := q.UpsertLink(ctx, store.UpsertLinkParams{MarketID: marketID, ItemKey: key, Ignored: true}); err != nil {
					return fmt.Errorf("ignore line: %w", err)
				}
				res.Ignored++
				continue
			}
			productID := it.ProductID
			if it.NewProduct != nil {
				p, err := s.createProduct(ctx, q, userID, *it.NewProduct)
				if err != nil {
					return err
				}
				productID = p.ID
			} else if _, err := getProduct(ctx, q, productID); err != nil {
				return err
			}
			if err := q.UpsertLink(ctx, store.UpsertLinkParams{MarketID: marketID, ItemKey: key, ProductID: sql.NullInt64{Int64: productID, Valid: true}}); err != nil {
				return fmt.Errorf("link line: %w", err)
			}
			if line.UnitPriceCents <= 0 {
				continue
			}
			if err := s.checkPrice(line.UnitPriceCents, rec.IssuedOn); err != nil {
				return err
			}
			if _, err := q.CreatePrice(ctx, store.CreatePriceParams{
				ProductID: productID, MarketID: marketID, PriceCents: line.UnitPriceCents, ObservedOn: rec.IssuedOn.String(), Source: SourceReceipt,
				ReceiptID: sql.NullInt64{Int64: receiptID, Valid: true}, CreatedBy: userID, CreatedAt: s.now().UTC(),
			}); err != nil {
				return fmt.Errorf("receipt price: %w", err)
			}
			res.Prices++
		}
		return nil
	})
	if err != nil {
		return ReceiptResult{}, err
	}
	s.cache.forget(in.URL)
	return res, nil
}

// receiptMarket returns the chosen market, giving it the receipt CNPJ, or creates the new one with it.
func (s *Service) receiptMarket(ctx context.Context, q *store.Queries, userID int64, in ReceiptCommit, cnpj string) (int64, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return 0, err
	}
	if in.NewMarket != nil {
		if cnpj != "" {
			if _, err := q.MarketByCNPJ(ctx, store.MarketByCNPJParams{GroupID: groupID, Cnpj: sql.NullString{String: cnpj, Valid: true}}); err == nil {
				cnpj = ""
			}
		}
		m, err := s.createMarket(ctx, q, userID, *in.NewMarket, cnpj)
		return m.ID, err
	}
	if in.MarketID == 0 {
		return 0, ErrMarketNotFound
	}
	if _, err := getMarket(ctx, q, in.MarketID); err != nil {
		return 0, err
	}
	if cnpj != "" {
		if _, err := q.MarketByCNPJ(ctx, store.MarketByCNPJParams{GroupID: groupID, Cnpj: sql.NullString{String: cnpj, Valid: true}}); errors.Is(err, sql.ErrNoRows) {
			if err := q.SetMarketCNPJ(ctx, store.SetMarketCNPJParams{Cnpj: sql.NullString{String: cnpj, Valid: true}, ID: in.MarketID, GroupID: groupID}); err != nil {
				return 0, fmt.Errorf("market cnpj: %w", err)
			}
		}
	}
	return in.MarketID, nil
}

// Receipts lists the receipts already read, newest first.
func (s *Service) Receipts(ctx context.Context) ([]ReceiptRecord, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.conn).ListReceipts(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list receipts: %w", err)
	}
	out := make([]ReceiptRecord, 0, len(rows))
	for _, r := range rows {
		d, err := civil.ParseDate(r.IssuedOn)
		if err != nil {
			return nil, err
		}
		out = append(out, ReceiptRecord{ID: r.ID, MarketName: r.MarketName, IssuedOn: d, TotalCents: r.TotalCents, ItemCount: r.ItemCount, PriceCount: r.PriceCount, CreatedByName: r.CreatedByName})
	}
	return out, nil
}

// DeleteReceipt removes a receipt and the prices it brought; the links stay learned.
func (s *Service) DeleteReceipt(ctx context.Context, id int64) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := q.GetReceipt(ctx, store.GetReceiptParams{ID: id, GroupID: groupID}); errors.Is(err, sql.ErrNoRows) {
			return ErrReceiptNotFound
		} else if err != nil {
			return err
		}
		return q.DeleteReceipt(ctx, store.DeleteReceiptParams{ID: id, GroupID: groupID})
	})
}

// receipt returns the download of the address, from the cache when recent.
func (s *Service) receipt(ctx context.Context, url string) (nfce.Receipt, error) {
	if rec, ok := s.cache.get(url, s.now()); ok {
		return rec, nil
	}
	rec, err := s.receipts.Fetch(ctx, url)
	if err != nil {
		return nfce.Receipt{}, err
	}
	s.cache.put(url, rec, s.now())
	return rec, nil
}

// get returns a recent download of the address.
func (c *receiptCache) get(url string, now time.Time) (nfce.Receipt, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[url]
	if !ok || now.Sub(it.at) > cacheTTL {
		return nfce.Receipt{}, false
	}
	return it.receipt, true
}

// put stores a download and drops the expired ones.
func (c *receiptCache) put(url string, rec nfce.Receipt, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, it := range c.items {
		if now.Sub(it.at) > cacheTTL {
			delete(c.items, k)
		}
	}
	c.items[url] = cachedReceipt{receipt: rec, at: now}
}

// forget drops a download once its receipt is recorded.
func (c *receiptCache) forget(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, url)
}

// itemKey identifies a receipt line inside a market: the printed code or, without one, the normalized text (RF-69).
func itemKey(it nfce.Item) string {
	if code := strings.TrimSpace(it.Code); code != "" {
		return "c:" + code
	}
	return "t:" + strings.ToLower(strings.Join(strings.Fields(it.Description), " "))
}
