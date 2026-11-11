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
