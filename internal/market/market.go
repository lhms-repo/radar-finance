// Package market manages markets, branded products, their price history, the
// shopping lists and the comparison between markets (RF-65 to RF-77, RN-22 to RN-26).
package market

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/civil"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/db/store"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/nfce"
	"github.com/italogfx/radar-finance/internal/problem"
)

// Unit is the unit of a product size.
type Unit string

// Product units; sizes are kept in thousandths of the unit.
const (
	UnitUnit       Unit = "un"
	UnitGram       Unit = "g"
	UnitKilogram   Unit = "kg"
	UnitMilliliter Unit = "ml"
	UnitLiter      Unit = "l"
)

// units are the accepted product units.
var units = map[Unit]bool{UnitUnit: true, UnitGram: true, UnitKilogram: true, UnitMilliliter: true, UnitLiter: true}

// Price sources: typed by hand or read from a receipt.
const (
	SourceManual  = "manual"
	SourceReceipt = "receipt"
)

// Limits of the text fields, in characters.
const (
	maxMarketName = 60
	maxProduct    = 60
	maxBrand      = 40
)

// Business errors returned by the service.
var (
	ErrMarketNotFound   = problem.NotFound("market_not_found", "Mercado não encontrado.")
	ErrMarketName       = problem.Invalid("market_name_invalid", "Informe o nome do mercado, com até 60 caracteres.")
	ErrMarketNeighbor   = problem.Invalid("market_neighborhood_invalid", "Use no máximo 60 caracteres no bairro.")
	ErrDuplicateMarket  = problem.Conflict("market_duplicate", "Já existe um mercado com esse nome nesse bairro.")
	ErrMarketInUse      = problem.Conflict("market_in_use", "Este mercado já tem preços. Desative-o para tirá-lo das comparações.")
	ErrProductNotFound  = problem.NotFound("product_not_found", "Produto não encontrado.")
	ErrProductName      = problem.Invalid("product_name_invalid", "Informe o nome do produto, com até 60 caracteres.")
	ErrProductBrand     = problem.Invalid("product_brand_invalid", "Informe a marca, com até 40 caracteres. Para itens a granel, use Sem marca.")
	ErrProductSize      = problem.Invalid("product_size_invalid", "Informe o tamanho do produto.")
	ErrProductUnit      = problem.Invalid("product_unit_invalid", "Escolha a unidade do produto.")
	ErrDuplicateProduct = problem.Conflict("product_duplicate", "Esse produto já está cadastrado.")
	ErrProductInUse     = problem.Conflict("product_in_use", "Este produto já tem preços ou está numa lista e não pode ser excluído.")
	ErrPriceNotFound    = problem.NotFound("price_not_found", "Preço não encontrado.")
	ErrPriceDate        = problem.Invalid("price_date_invalid", "Informe uma data até hoje.")
	ErrSettingsMarkets  = problem.Invalid("market_settings_markets", "Use de 1 a 5 mercados por compra.")
	ErrSettingsSavings  = problem.Invalid("market_settings_savings", "Informe uma economia mínima de zero para cima.")
	ErrSettingsDays     = problem.Invalid("market_settings_days", "Use um prazo de 1 a 365 dias.")
)

// Service holds the market rules.
type Service struct {
	conn     *sql.DB
	now      func() time.Time
	today    func() civil.Date
	receipts Fetcher
	cache    *receiptCache
	payments payments
}

// NewService returns the service with the real clock in São Paulo.
func NewService(conn *sql.DB) *Service {
	loc, err := time.LoadLocation(config.Location)
	if err != nil {
		loc = time.UTC
	}
	return &Service{conn: conn, now: time.Now, today: func() civil.Date { return civil.Today(loc) }, receipts: nfce.NewClient(), cache: newReceiptCache(),
		payments: payments{ledger: ledger.NewService(conn), cards: card.NewService(conn), alelo: alelo.NewService(conn)}}
}

// Market is a store where prices are taken (RF-65).
type Market struct {
	ID           int64
	Name         string
	Neighborhood string
	CNPJ         string
	Active       bool
	PriceCount   int64
}

// MarketInput registers or edits a market; Active is ignored on registration.
type MarketInput struct {
	Name         string
	Neighborhood string
	Active       bool
}

// Product is always one brand in one size; ByWeight products are priced per kg (RF-66).
type Product struct {
	ID        int64
	Name      string
	Brand     string
	ByWeight  bool
	SizeMilli int64
	Unit      Unit
}

// ProductInput registers or edits a product.
type ProductInput struct {
	Name      string
	Brand     string
	ByWeight  bool
	SizeMilli int64
	Unit      Unit
}

// Price is one observation of a product price in a market (RF-67).
type Price struct {
	ID           int64
	ProductID    int64
	MarketID     int64
	MarketName   string
	Neighborhood string
	PriceCents   int64
	Date         civil.Date
	Source       string
}

// PriceInput records a price typed by hand (RF-70).
type PriceInput struct {
	ProductID  int64
	MarketID   int64
	PriceCents int64
	Date       civil.Date
}

// PriceChanges corrects a recorded price (RF-70).
type PriceChanges struct {
	PriceCents int64
	Date       civil.Date
}

// Key identifies the latest price of a product in a market.
type Key struct {
	ProductID int64
	MarketID  int64
}

// Settings are the comparison settings shared by the couple (RF-74, RN-25).
type Settings struct {
	MaxMarkets      int
	MinSavingsCents int64
	StaleDays       int
}

// Label is the product name as shown and compared: name, brand and size, or "por kg" (RF-66).
func (p Product) Label() string {
	if p.ByWeight {
		return fmt.Sprintf("%s %s, por kg", p.Name, p.Brand)
	}
	return fmt.Sprintf("%s %s %s", p.Name, p.Brand, FormatSize(p.SizeMilli, p.Unit))
}

// FormatSize writes a size in thousandths with a comma, as 500 g or 1,5 L.
func FormatSize(milli int64, u Unit) string {
	whole, frac := milli/1000, milli%1000
	s := strconv.FormatInt(whole, 10)
	if frac != 0 {
		s += "," + strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
	}
	unit := string(u)
	if u == UnitLiter {
		unit = "L"
	}
	return s + " " + unit
}

// WithFetcher replaces the receipt reader, for tests and the fixtures of the end to end tests.
func (s *Service) WithFetcher(f Fetcher) *Service {
	s.receipts = f
	return s
}

// Markets lists the markets, active first.
func (s *Service) Markets(ctx context.Context) ([]Market, error) {
	rows, err := store.New(s.conn).ListMarkets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list markets: %w", err)
	}
	out := make([]Market, 0, len(rows))
	for _, r := range rows {
		out = append(out, Market{ID: r.ID, Name: r.Name, Neighborhood: r.Neighborhood, CNPJ: r.Cnpj.String, Active: r.Active, PriceCount: r.PriceCount})
	}
	return out, nil
}

// CreateMarket registers a market (RF-65).
func (s *Service) CreateMarket(ctx context.Context, userID int64, in MarketInput) (Market, error) {
	return s.createMarket(ctx, store.New(s.conn), userID, in, "")
}

// createMarket registers a market inside a transaction, with the CNPJ read from a receipt when known.
func (s *Service) createMarket(ctx context.Context, q *store.Queries, userID int64, in MarketInput, cnpj string) (Market, error) {
	name, neighborhood, err := cleanMarket(in)
	if err != nil {
		return Market{}, err
	}
	if err := checkUniqueMarket(ctx, q, name, neighborhood, 0); err != nil {
		return Market{}, err
	}
	m, err := q.CreateMarket(ctx, store.CreateMarketParams{
		Name: name, Neighborhood: neighborhood, Cnpj: sql.NullString{String: cnpj, Valid: cnpj != ""}, CreatedBy: userID, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return Market{}, fmt.Errorf("create market: %w", err)
	}
	return Market{ID: m.ID, Name: m.Name, Neighborhood: m.Neighborhood, CNPJ: cnpj, Active: m.Active}, nil
}

// UpdateMarket renames or deactivates a market.
func (s *Service) UpdateMarket(ctx context.Context, id int64, in MarketInput) (Market, error) {
	name, neighborhood, err := cleanMarket(in)
	if err != nil {
		return Market{}, err
	}
	var out Market
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		m, err := getMarket(ctx, q, id)
		if err != nil {
			return err
		}
		if err := checkUniqueMarket(ctx, q, name, neighborhood, id); err != nil {
			return err
		}
		if err := q.UpdateMarket(ctx, store.UpdateMarketParams{Name: name, Neighborhood: neighborhood, Active: in.Active, ID: id}); err != nil {
			return fmt.Errorf("update market: %w", err)
		}
		out = Market{ID: id, Name: name, Neighborhood: neighborhood, CNPJ: m.Cnpj.String, Active: in.Active}
		return nil
	})
	return out, err
}

// DeleteMarket removes a market without prices.
func (s *Service) DeleteMarket(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getMarket(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteMarket(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrMarketInUse
			}
			return fmt.Errorf("delete market: %w", err)
		}
		return nil
	})
}

// Products lists every product by name.
func (s *Service) Products(ctx context.Context) ([]Product, error) {
	rows, err := store.New(s.conn).ListProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	out := make([]Product, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProduct(r))
	}
	return out, nil
}

// CreateProduct registers a branded product (RF-66).
func (s *Service) CreateProduct(ctx context.Context, userID int64, in ProductInput) (Product, error) {
	var out Product
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		p, err := s.createProduct(ctx, q, userID, in)
		out = p
		return err
	})
	return out, err
}

// createProduct registers a product inside a transaction.
func (s *Service) createProduct(ctx context.Context, q *store.Queries, userID int64, in ProductInput) (Product, error) {
	in, err := cleanProduct(in)
	if err != nil {
		return Product{}, err
	}
	if err := checkUniqueProduct(ctx, q, in, 0); err != nil {
		return Product{}, err
	}
	r, err := q.CreateProduct(ctx, store.CreateProductParams{
		Name: in.Name, Brand: in.Brand, ByWeight: in.ByWeight, SizeMilli: in.SizeMilli, Unit: string(in.Unit), CreatedBy: userID, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	return toProduct(r), nil
}

// UpdateProduct edits a product.
func (s *Service) UpdateProduct(ctx context.Context, id int64, in ProductInput) (Product, error) {
	in, err := cleanProduct(in)
	if err != nil {
		return Product{}, err
	}
	err = db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getProduct(ctx, q, id); err != nil {
			return err
		}
		if err := checkUniqueProduct(ctx, q, in, id); err != nil {
			return err
		}
		return q.UpdateProduct(ctx, store.UpdateProductParams{Name: in.Name, Brand: in.Brand, ByWeight: in.ByWeight, SizeMilli: in.SizeMilli, Unit: string(in.Unit), ID: id})
	})
	if err != nil {
		return Product{}, err
	}
	return Product{ID: id, Name: in.Name, Brand: in.Brand, ByWeight: in.ByWeight, SizeMilli: in.SizeMilli, Unit: in.Unit}, nil
}

// DeleteProduct removes a product without prices or list items.
func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if _, err := getProduct(ctx, q, id); err != nil {
			return err
		}
		if err := q.DeleteProduct(ctx, id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return ErrProductInUse
			}
			return fmt.Errorf("delete product: %w", err)
		}
		return nil
	})
}

// AddPrice records a price typed by hand (RF-67, RF-70).
func (s *Service) AddPrice(ctx context.Context, userID int64, in PriceInput) (int64, error) {
	var id int64
	err := db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		var err error
		id, err = s.addPrice(ctx, q, userID, in, SourceManual)
		return err
	})
	return id, err
}

// addPrice validates and records a price inside a transaction.
func (s *Service) addPrice(ctx context.Context, q *store.Queries, userID int64, in PriceInput, source string) (int64, error) {
	if err := s.checkPrice(in.PriceCents, in.Date); err != nil {
		return 0, err
	}
	if _, err := getProduct(ctx, q, in.ProductID); err != nil {
		return 0, err
	}
	if _, err := getMarket(ctx, q, in.MarketID); err != nil {
		return 0, err
	}
	id, err := q.CreatePrice(ctx, store.CreatePriceParams{
		ProductID: in.ProductID, MarketID: in.MarketID, PriceCents: in.PriceCents, ObservedOn: in.Date.String(), Source: source, CreatedBy: userID, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		return 0, fmt.Errorf("create price: %w", err)
	}
	return id, nil
}

// UpdatePrice corrects the value or the date of a price (RF-70).
func (s *Service) UpdatePrice(ctx context.Context, id int64, c PriceChanges) error {
	if err := s.checkPrice(c.PriceCents, c.Date); err != nil {
		return err
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkPriceExists(ctx, q, id); err != nil {
			return err
		}
		return q.UpdatePrice(ctx, store.UpdatePriceParams{PriceCents: c.PriceCents, ObservedOn: c.Date.String(), ID: id})
	})
}

// DeletePrice removes a wrong price (RF-70).
func (s *Service) DeletePrice(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		if err := checkPriceExists(ctx, q, id); err != nil {
			return err
		}
		return q.DeletePrice(ctx, id)
	})
}

// History lists the prices of a product in every market, newest first (RF-76).
func (s *Service) History(ctx context.Context, productID int64) ([]Price, error) {
	q := store.New(s.conn)
	if _, err := getProduct(ctx, q, productID); err != nil {
		return nil, err
	}
	rows, err := q.ProductPrices(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("product prices: %w", err)
	}
	out := make([]Price, 0, len(rows))
	for _, r := range rows {
		d, err := civil.ParseDate(r.ObservedOn)
		if err != nil {
			return nil, err
		}
		out = append(out, Price{ID: r.ID, ProductID: productID, MarketID: r.MarketID, MarketName: r.MarketName, Neighborhood: r.Neighborhood, PriceCents: r.PriceCents, Date: d, Source: r.Source})
	}
	return out, nil
}

// Latest returns the latest price of every product in every market (RN-22).
func (s *Service) Latest(ctx context.Context) (map[Key]Price, error) {
	return latest(ctx, store.New(s.conn))
}

// latest reads the latest prices with the given queries.
func latest(ctx context.Context, q *store.Queries) (map[Key]Price, error) {
	rows, err := q.LatestPrices(ctx)
	if err != nil {
		return nil, fmt.Errorf("latest prices: %w", err)
	}
	out := make(map[Key]Price, len(rows))
	for _, r := range rows {
		d, err := civil.ParseDate(r.ObservedOn)
		if err != nil {
			return nil, err
		}
		out[Key{ProductID: r.ProductID, MarketID: r.MarketID}] = Price{ID: r.ID, ProductID: r.ProductID, MarketID: r.MarketID, PriceCents: r.PriceCents, Date: d, Source: r.Source}
	}
	return out, nil
}

// Settings reads the comparison settings (RF-74, RN-25).
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	return settings(ctx, store.New(s.conn))
}

// settings reads the comparison settings with the given queries.
func settings(ctx context.Context, q *store.Queries) (Settings, error) {
	r, err := q.GetMarketSettings(ctx)
	if err != nil {
		return Settings{}, fmt.Errorf("market settings: %w", err)
	}
	return Settings{MaxMarkets: int(r.MaxMarkets), MinSavingsCents: r.MinSavingsCents, StaleDays: int(r.StaleDays)}, nil
}

// UpdateSettings saves the comparison settings (RF-74, RN-25).
func (s *Service) UpdateSettings(ctx context.Context, in Settings) error {
	switch {
	case in.MaxMarkets < 1 || in.MaxMarkets > 5:
		return ErrSettingsMarkets
	case in.MinSavingsCents < 0 || in.MinSavingsCents > ledger.MaxAmountCents:
		return ErrSettingsSavings
	case in.StaleDays < 1 || in.StaleDays > 365:
		return ErrSettingsDays
	}
	return db.WithTx(ctx, s.conn, func(q *store.Queries) error {
		return q.UpdateMarketSettings(ctx, store.UpdateMarketSettingsParams{MaxMarkets: int64(in.MaxMarkets), MinSavingsCents: in.MinSavingsCents, StaleDays: int64(in.StaleDays)})
	})
}

// checkPrice requires a positive price on a date up to today.
func (s *Service) checkPrice(cents int64, d civil.Date) error {
	if err := ledger.CheckPositive(cents); err != nil {
		return err
	}
	if d.IsZero() || s.today().Before(d) {
		return ErrPriceDate
	}
	return nil
}

// cleanMarket trims and bounds the name and the neighborhood.
func cleanMarket(in MarketInput) (string, string, error) {
	name, neighborhood := strings.TrimSpace(in.Name), strings.TrimSpace(in.Neighborhood)
	if name == "" || utf8.RuneCountInString(name) > maxMarketName {
		return "", "", ErrMarketName
	}
	if utf8.RuneCountInString(neighborhood) > maxMarketName {
		return "", "", ErrMarketNeighbor
	}
	return name, neighborhood, nil
}

// cleanProduct trims the texts and makes products sold by weight priced per kg.
func cleanProduct(in ProductInput) (ProductInput, error) {
	in.Name, in.Brand = strings.Join(strings.Fields(in.Name), " "), strings.Join(strings.Fields(in.Brand), " ")
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > maxProduct:
		return in, ErrProductName
	case in.Brand == "" || utf8.RuneCountInString(in.Brand) > maxBrand:
		return in, ErrProductBrand
	}
	if in.ByWeight {
		in.SizeMilli, in.Unit = 1_000, UnitKilogram
		return in, nil
	}
	if !units[in.Unit] {
		return in, ErrProductUnit
	}
	if in.SizeMilli <= 0 || in.SizeMilli > 1_000_000_000 {
		return in, ErrProductSize
	}
	return in, nil
}

// checkUniqueMarket rejects another market with the same name and neighborhood.
func checkUniqueMarket(ctx context.Context, q *store.Queries, name, neighborhood string, self int64) error {
	rows, err := q.ListMarkets(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != self && strings.EqualFold(r.Name, name) && strings.EqualFold(r.Neighborhood, neighborhood) {
			return ErrDuplicateMarket
		}
	}
	return nil
}

// checkUniqueProduct rejects another product with the same name, brand and size.
func checkUniqueProduct(ctx context.Context, q *store.Queries, in ProductInput, self int64) error {
	rows, err := q.ListProducts(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != self && strings.EqualFold(r.Name, in.Name) && strings.EqualFold(r.Brand, in.Brand) &&
			r.ByWeight == in.ByWeight && r.SizeMilli == in.SizeMilli && r.Unit == string(in.Unit) {
			return ErrDuplicateProduct
		}
	}
	return nil
}

// getMarket reads one market.
func getMarket(ctx context.Context, q *store.Queries, id int64) (store.Market, error) {
	m, err := q.GetMarket(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrMarketNotFound
	}
	return m, err
}

// getProduct reads one product.
func getProduct(ctx context.Context, q *store.Queries, id int64) (Product, error) {
	r, err := q.GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, err
	}
	return toProduct(r), nil
}

// checkPriceExists fails when the price is missing.
func checkPriceExists(ctx context.Context, q *store.Queries, id int64) error {
	if _, err := q.GetPrice(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return ErrPriceNotFound
	} else if err != nil {
		return err
	}
	return nil
}

// toProduct converts a product row.
func toProduct(r store.Product) Product {
	return Product{ID: r.ID, Name: r.Name, Brand: r.Brand, ByWeight: r.ByWeight, SizeMilli: r.SizeMilli, Unit: Unit(r.Unit)}
}

// CatalogItem is a product with its latest price in each market (RN-22).
type CatalogItem struct {
	Product
	Prices []LatestPrice
}

// LatestPrice is the latest price of a product in one market, flagged when older than the configured days (RN-25).
type LatestPrice struct {
	MarketID   int64
	PriceCents int64
	Date       civil.Date
	Stale      bool
}

// Catalog lists every product with the latest price in each market, markets in name order (RF-75, RN-22, RN-25).
func (s *Service) Catalog(ctx context.Context) ([]CatalogItem, error) {
	q := store.New(s.conn)
	products, err := s.Products(ctx)
	if err != nil {
		return nil, err
	}
	markets, err := s.Markets(ctx)
	if err != nil {
		return nil, err
	}
	prices, err := latest(ctx, q)
	if err != nil {
		return nil, err
	}
	cfg, err := settings(ctx, q)
	if err != nil {
		return nil, err
	}
	limit := s.today().AddDays(-cfg.StaleDays)
	out := make([]CatalogItem, 0, len(products))
	for _, p := range products {
		item := CatalogItem{Product: p, Prices: []LatestPrice{}}
		for _, m := range markets {
			if pr, ok := prices[Key{ProductID: p.ID, MarketID: m.ID}]; ok {
				item.Prices = append(item.Prices, LatestPrice{MarketID: m.ID, PriceCents: pr.PriceCents, Date: pr.Date, Stale: pr.Date.Before(limit)})
			}
		}
		out = append(out, item)
	}
	return out, nil
}
