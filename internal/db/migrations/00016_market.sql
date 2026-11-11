-- Markets, branded products and their price history (RF-65 to RF-67, RF-70,
-- RF-74, RF-76, RN-25).
--
-- A product is always a specific brand and size; items sold by weight are
-- products with by_weight set, priced per kg (RF-66). Sizes are kept in
-- thousandths of the unit so 1,5 L needs no floating point. Prices are cents
-- (RN-21), truncated when the receipt brings three decimals. The latest price
-- of a product in a market is the one with the most recent date, then the
-- newest row (RN-22).

-- +goose Up
CREATE TABLE markets (
    id           INTEGER  PRIMARY KEY,
    name         TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    neighborhood TEXT     NOT NULL DEFAULT '' CHECK (length(neighborhood) <= 60),
    cnpj         TEXT     UNIQUE CHECK (cnpj IS NULL OR length(cnpj) = 14),
    active       BOOLEAN  NOT NULL DEFAULT 1,
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL
);

CREATE UNIQUE INDEX markets_name_neighborhood ON markets (name COLLATE NOCASE, neighborhood COLLATE NOCASE);

CREATE TABLE products (
    id         INTEGER  PRIMARY KEY,
    name       TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    brand      TEXT     NOT NULL CHECK (length(trim(brand)) > 0 AND length(brand) <= 40),
    by_weight  BOOLEAN  NOT NULL DEFAULT 0,
    size_milli INTEGER  NOT NULL CHECK (size_milli > 0),
    unit       TEXT     NOT NULL CHECK (unit IN ('un', 'g', 'kg', 'ml', 'l')),
    created_by INTEGER  NOT NULL REFERENCES users (id),
    created_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX products_identity ON products (name COLLATE NOCASE, brand COLLATE NOCASE, by_weight, size_milli, unit);

CREATE TABLE prices (
    id          INTEGER  PRIMARY KEY,
    product_id  INTEGER  NOT NULL REFERENCES products (id),
    market_id   INTEGER  NOT NULL REFERENCES markets (id),
    price_cents INTEGER  NOT NULL CHECK (price_cents > 0),
    observed_on TEXT     NOT NULL,
    source      TEXT     NOT NULL CHECK (source IN ('manual', 'receipt')),
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL
);

CREATE INDEX prices_latest ON prices (product_id, market_id, observed_on, id);

-- One row with the comparison settings shared by the couple (RF-74, RN-25).
CREATE TABLE market_settings (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    max_markets       INTEGER NOT NULL CHECK (max_markets BETWEEN 1 AND 5),
    min_savings_cents INTEGER NOT NULL CHECK (min_savings_cents >= 0),
    stale_days        INTEGER NOT NULL CHECK (stale_days BETWEEN 1 AND 365)
);

INSERT INTO market_settings (id, max_markets, min_savings_cents, stale_days) VALUES (1, 2, 1000, 30);

-- +goose Down
DROP TABLE market_settings;
DROP TABLE prices;
DROP TABLE products;
DROP TABLE markets;
