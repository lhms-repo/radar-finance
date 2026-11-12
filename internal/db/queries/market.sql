-- name: ListMarkets :many
SELECT m.*, CAST((SELECT count(*) FROM prices p WHERE p.market_id = m.id) AS INTEGER) AS price_count
FROM markets m
WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY m.active DESC, m.name COLLATE NOCASE, m.neighborhood COLLATE NOCASE;

-- name: GetMarket :one
SELECT *
FROM markets
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateMarket :one
INSERT INTO markets (name, neighborhood, cnpj, created_by, created_at, group_id)
VALUES (?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: UpdateMarket :exec
UPDATE markets
SET name = ?, neighborhood = ?, active = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteMarket :exec
DELETE FROM markets
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: ListProducts :many
SELECT *
FROM products
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY name COLLATE NOCASE, brand COLLATE NOCASE, size_milli;

-- name: GetProduct :one
SELECT *
FROM products
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateProduct :one
INSERT INTO products (name, brand, by_weight, size_milli, unit, created_by, created_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: UpdateProduct :exec
UPDATE products
SET name = ?, brand = ?, by_weight = ?, size_milli = ?, unit = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteProduct :exec
DELETE FROM products
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: LatestPrices :many
-- Latest price of every product in every market: most recent date, then newest row (RN-22).
SELECT p.id, p.product_id, p.market_id, p.price_cents, p.observed_on, p.source
FROM prices p
JOIN markets pm ON pm.id = p.market_id
WHERE pm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND p.id = (
    SELECT p2.id FROM prices p2
    WHERE p2.product_id = p.product_id AND p2.market_id = p.market_id
    ORDER BY p2.observed_on DESC, p2.id DESC
    LIMIT 1
);

-- name: ProductPrices :many
-- Price history of one product in every market, newest first (RF-76).
SELECT p.id, p.market_id, m.name AS market_name, m.neighborhood, p.price_cents, p.observed_on, p.source
FROM prices p
JOIN markets m ON m.id = p.market_id
WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND p.product_id = sqlc.arg(product_id)
ORDER BY p.observed_on DESC, p.id DESC;

-- name: GetPrice :one
SELECT p.*
FROM prices p
JOIN markets m ON m.id = p.market_id
WHERE p.id = sqlc.arg(id) AND m.group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreatePrice :one
INSERT INTO prices (product_id, market_id, price_cents, observed_on, source, receipt_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdatePrice :exec
UPDATE prices
SET price_cents = ?, observed_on = ?
WHERE prices.id = sqlc.arg(id)
  AND market_id IN (SELECT m.id FROM markets m WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER));

-- name: DeletePrice :exec
DELETE FROM prices
WHERE prices.id = sqlc.arg(id)
  AND market_id IN (SELECT m.id FROM markets m WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER));

-- name: GetMarketSettings :one
-- The row of the group; none means the defaults apply (RF-74).
SELECT *
FROM market_settings
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: UpsertMarketSettings :exec
INSERT INTO market_settings (group_id, max_markets, min_savings_cents, stale_days)
VALUES (CAST(sqlc.arg(group_id) AS INTEGER), sqlc.arg(max_markets), sqlc.arg(min_savings_cents), sqlc.arg(stale_days))
ON CONFLICT (group_id) DO UPDATE SET max_markets = excluded.max_markets,
    min_savings_cents = excluded.min_savings_cents, stale_days = excluded.stale_days;

-- name: MarketByCNPJ :one
SELECT *
FROM markets
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND cnpj = sqlc.arg(cnpj);

-- name: SetMarketCNPJ :exec
UPDATE markets
SET cnpj = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND cnpj IS NULL;

-- name: ReceiptByKey :one
SELECT r.id
FROM receipts r
JOIN markets m ON m.id = r.market_id
WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND r.access_key = sqlc.arg(access_key);

-- name: CreateReceipt :one
INSERT INTO receipts (market_id, access_key, issued_on, total_cents, item_count, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: ListReceipts :many
SELECT r.id, r.issued_on, r.total_cents, r.item_count, m.name AS market_name, pe.name AS created_by_name,
       CAST((SELECT count(*) FROM prices p WHERE p.receipt_id = r.id) AS INTEGER) AS price_count
FROM receipts r
JOIN markets m ON m.id = r.market_id
JOIN users u ON u.id = r.created_by
JOIN persons pe ON pe.id = u.person_id
WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY r.issued_on DESC, r.id DESC;

-- name: GetReceipt :one
SELECT r.*
FROM receipts r
JOIN markets m ON m.id = r.market_id
WHERE r.id = sqlc.arg(id) AND m.group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteReceipt :exec
DELETE FROM receipts
WHERE receipts.id = sqlc.arg(id)
  AND market_id IN (SELECT m.id FROM markets m WHERE m.group_id = CAST(sqlc.arg(group_id) AS INTEGER));

-- name: MarketLinks :many
SELECT item_key, product_id, ignored
FROM receipt_links
WHERE market_id = ?;

-- name: UpsertLink :exec
INSERT INTO receipt_links (market_id, item_key, product_id, ignored)
VALUES (?, ?, ?, ?)
ON CONFLICT (market_id, item_key) DO UPDATE SET product_id = excluded.product_id, ignored = excluded.ignored;
