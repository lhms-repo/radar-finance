package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/market"
)

// ListMarkets lists the markets (RF-65).
func (a *API) ListMarkets(ctx context.Context, _ gen.ListMarketsRequestObject) (gen.ListMarketsResponseObject, error) {
	ms, err := a.d.Market.Markets(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListMarkets200JSONResponse, 0, len(ms))
	for _, m := range ms {
		out = append(out, toMarket(m))
	}
	return out, nil
}

// CreateMarket registers a market (RF-65).
func (a *API) CreateMarket(ctx context.Context, req gen.CreateMarketRequestObject) (gen.CreateMarketResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	m, err := a.d.Market.CreateMarket(ctx, auth.UserID(ctx), market.MarketInput{Name: req.Body.Name, Neighborhood: req.Body.Neighborhood})
	if err != nil {
		return nil, err
	}
	return gen.CreateMarket201JSONResponse(toMarket(m)), nil
}

// UpdateMarket renames or deactivates a market.
func (a *API) UpdateMarket(ctx context.Context, req gen.UpdateMarketRequestObject) (gen.UpdateMarketResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	active := req.Body.Active == nil || *req.Body.Active
	m, err := a.d.Market.UpdateMarket(ctx, req.Id, market.MarketInput{Name: req.Body.Name, Neighborhood: req.Body.Neighborhood, Active: active})
	if err != nil {
		return nil, err
	}
	return gen.UpdateMarket200JSONResponse(toMarket(m)), nil
}

// DeleteMarket removes a market without prices.
func (a *API) DeleteMarket(ctx context.Context, req gen.DeleteMarketRequestObject) (gen.DeleteMarketResponseObject, error) {
	if err := a.d.Market.DeleteMarket(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteMarket204Response{}, nil
}

// ListProducts lists the products with the latest price in each market (RF-66, RN-22, RN-25).
func (a *API) ListProducts(ctx context.Context, _ gen.ListProductsRequestObject) (gen.ListProductsResponseObject, error) {
	items, err := a.d.Market.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListProducts200JSONResponse, 0, len(items))
	for _, it := range items {
		p := toProduct(it.Product)
		for _, pr := range it.Prices {
			p.Prices = append(p.Prices, gen.LatestPrice{MarketId: pr.MarketID, PriceCents: pr.PriceCents, Date: apiDate(pr.Date), Stale: pr.Stale})
		}
		out = append(out, p)
	}
	return out, nil
}

// CreateProduct registers a branded product (RF-66).
func (a *API) CreateProduct(ctx context.Context, req gen.CreateProductRequestObject) (gen.CreateProductResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Market.CreateProduct(ctx, auth.UserID(ctx), fromProductInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return gen.CreateProduct201JSONResponse(toProduct(p)), nil
}

// UpdateProduct edits a product.
func (a *API) UpdateProduct(ctx context.Context, req gen.UpdateProductRequestObject) (gen.UpdateProductResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	p, err := a.d.Market.UpdateProduct(ctx, req.Id, fromProductInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return gen.UpdateProduct200JSONResponse(toProduct(p)), nil
}

// DeleteProduct removes a product without prices or list items.
func (a *API) DeleteProduct(ctx context.Context, req gen.DeleteProductRequestObject) (gen.DeleteProductResponseObject, error) {
	if err := a.d.Market.DeleteProduct(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteProduct204Response{}, nil
}

// ListProductPrices returns the price history of a product (RF-76).
func (a *API) ListProductPrices(ctx context.Context, req gen.ListProductPricesRequestObject) (gen.ListProductPricesResponseObject, error) {
	prices, err := a.d.Market.History(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListProductPrices200JSONResponse, 0, len(prices))
	for _, p := range prices {
		out = append(out, gen.MarketPrice{
			Id: p.ID, MarketId: p.MarketID, MarketName: p.MarketName, Neighborhood: p.Neighborhood, PriceCents: p.PriceCents,
			Date: apiDate(p.Date), Source: gen.MarketPriceSource(p.Source),
		})
	}
	return out, nil
}

// CreatePrice records a price typed by hand (RF-67, RF-70).
func (a *API) CreatePrice(ctx context.Context, req gen.CreatePriceRequestObject) (gen.CreatePriceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	if _, err := a.d.Market.AddPrice(ctx, auth.UserID(ctx), market.PriceInput{ProductID: b.ProductId, MarketID: b.MarketId, PriceCents: b.PriceCents, Date: fromDate(b.Date)}); err != nil {
		return nil, err
	}
	return gen.CreatePrice204Response{}, nil
}

// UpdatePrice corrects a price (RF-70).
func (a *API) UpdatePrice(ctx context.Context, req gen.UpdatePriceRequestObject) (gen.UpdatePriceResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	if err := a.d.Market.UpdatePrice(ctx, req.Id, market.PriceChanges{PriceCents: req.Body.PriceCents, Date: fromDate(req.Body.Date)}); err != nil {
		return nil, err
	}
	return gen.UpdatePrice204Response{}, nil
}

// DeletePrice removes a wrong price (RF-70).
func (a *API) DeletePrice(ctx context.Context, req gen.DeletePriceRequestObject) (gen.DeletePriceResponseObject, error) {
	if err := a.d.Market.DeletePrice(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeletePrice204Response{}, nil
}

// GetMarketSettings returns the comparison settings (RF-74, RN-25).
func (a *API) GetMarketSettings(ctx context.Context, _ gen.GetMarketSettingsRequestObject) (gen.GetMarketSettingsResponseObject, error) {
	s, err := a.d.Market.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetMarketSettings200JSONResponse{MaxMarkets: s.MaxMarkets, MinSavingsCents: s.MinSavingsCents, StaleDays: s.StaleDays}, nil
}

// UpdateMarketSettings saves the comparison settings (RF-74, RN-25).
func (a *API) UpdateMarketSettings(ctx context.Context, req gen.UpdateMarketSettingsRequestObject) (gen.UpdateMarketSettingsResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	if err := a.d.Market.UpdateSettings(ctx, market.Settings{MaxMarkets: b.MaxMarkets, MinSavingsCents: b.MinSavingsCents, StaleDays: b.StaleDays}); err != nil {
		return nil, err
	}
	return gen.UpdateMarketSettings204Response{}, nil
}

// toMarket converts a market to the API model.
func toMarket(m market.Market) gen.Market {
	return gen.Market{Id: m.ID, Name: m.Name, Neighborhood: m.Neighborhood, Active: m.Active, PriceCount: m.PriceCount}
}

// toProduct converts a product to the API model, without prices.
func toProduct(p market.Product) gen.MarketProduct {
	return gen.MarketProduct{
		Id: p.ID, Name: p.Name, Brand: p.Brand, ByWeight: p.ByWeight, SizeMilli: p.SizeMilli, Unit: gen.ProductUnit(p.Unit),
		Label: p.Label(), Prices: []gen.LatestPrice{},
	}
}

// fromProductInput converts the API product input.
func fromProductInput(b gen.ProductInput) market.ProductInput {
	return market.ProductInput{Name: b.Name, Brand: b.Brand, ByWeight: b.ByWeight, SizeMilli: b.SizeMilli, Unit: market.Unit(b.Unit)}
}

// PreviewReceipt downloads the receipt of the QR code and shows the learned links (RF-68, RF-69).
func (a *API) PreviewReceipt(ctx context.Context, req gen.PreviewReceiptRequestObject) (gen.PreviewReceiptResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	var marketID int64
	if req.Body.MarketId != nil {
		marketID = *req.Body.MarketId
	}
	p, err := a.d.Market.PreviewReceipt(ctx, req.Body.Url, marketID)
	if err != nil {
		return nil, err
	}
	out := gen.PreviewReceipt200JSONResponse{
		Url: p.URL, AccessKey: p.AccessKey, StoreName: p.StoreName, Neighborhood: p.Neighborhood, TotalCents: p.TotalCents,
		MarketId: p.MarketID, ImportedId: p.ImportedID, Items: make([]gen.ReceiptLine, 0, len(p.Items)),
	}
	if !p.IssuedOn.IsZero() {
		d := apiDate(p.IssuedOn)
		out.IssuedOn = &d
	}
	for _, it := range p.Items {
		out.Items = append(out.Items, gen.ReceiptLine{
			Index: it.Index, Description: it.Description, Code: it.Code, QuantityMilli: it.QuantityMilli, Unit: it.Unit,
			UnitPriceCents: it.UnitPriceCents, TotalCents: it.TotalCents, ProductId: it.ProductID, Ignored: it.Ignored,
		})
	}
	return out, nil
}

// CommitReceipt records the receipt prices and the learned links (RF-68, RF-69).
func (a *API) CommitReceipt(ctx context.Context, req gen.CommitReceiptRequestObject) (gen.CommitReceiptResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	b := req.Body
	in := market.ReceiptCommit{URL: b.Url, Items: make([]market.CommitItem, 0, len(b.Items))}
	if b.MarketId != nil {
		in.MarketID = *b.MarketId
	}
	if b.NewMarket != nil {
		in.NewMarket = &market.MarketInput{Name: b.NewMarket.Name, Neighborhood: b.NewMarket.Neighborhood}
	}
	for _, it := range b.Items {
		c := market.CommitItem{Index: it.Index, Ignore: it.Ignore != nil && *it.Ignore}
		if it.ProductId != nil {
			c.ProductID = *it.ProductId
		}
		if it.NewProduct != nil {
			p := fromProductInput(*it.NewProduct)
			c.NewProduct = &p
		}
		in.Items = append(in.Items, c)
	}
	res, err := a.d.Market.CommitReceipt(ctx, auth.UserID(ctx), in)
	if err != nil {
		return nil, err
	}
	return gen.CommitReceipt201JSONResponse{ReceiptId: res.ReceiptID, MarketId: res.MarketID, Prices: res.Prices, Ignored: res.Ignored}, nil
}

// ListReceipts returns the receipts already read.
func (a *API) ListReceipts(ctx context.Context, _ gen.ListReceiptsRequestObject) (gen.ListReceiptsResponseObject, error) {
	rs, err := a.d.Market.Receipts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListReceipts200JSONResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, gen.ReceiptRecord{
			Id: r.ID, MarketName: r.MarketName, IssuedOn: apiDate(r.IssuedOn), TotalCents: r.TotalCents,
			ItemCount: r.ItemCount, PriceCount: r.PriceCount, CreatedByName: r.CreatedByName,
		})
	}
	return out, nil
}

// DeleteReceipt removes a receipt and its prices.
func (a *API) DeleteReceipt(ctx context.Context, req gen.DeleteReceiptRequestObject) (gen.DeleteReceiptResponseObject, error) {
	if err := a.d.Market.DeleteReceipt(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteReceipt204Response{}, nil
}
