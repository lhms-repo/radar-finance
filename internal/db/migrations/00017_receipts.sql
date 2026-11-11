-- Consumer receipts (NFC-e) read from the QR code and the link of each receipt
-- line to a product, per market (RF-68, RF-69).
--
-- A receipt is identified by its 44 digit access key and enters only once. Its
-- prices point to it, so removing a receipt removes its prices. A line is
-- recognized by the code printed on the receipt or, without a code, by its
-- normalized text; the link either names the product or tells to ignore the
-- line, like a bag.

-- +goose Up
CREATE TABLE receipts (
    id          INTEGER  PRIMARY KEY,
    market_id   INTEGER  NOT NULL REFERENCES markets (id),
    access_key  TEXT     NOT NULL UNIQUE CHECK (length(access_key) = 44),
    issued_on   TEXT     NOT NULL,
    total_cents INTEGER  NOT NULL CHECK (total_cents >= 0),
    item_count  INTEGER  NOT NULL CHECK (item_count >= 0),
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL
);

ALTER TABLE prices ADD COLUMN receipt_id INTEGER REFERENCES receipts (id) ON DELETE CASCADE;

CREATE TABLE receipt_links (
    id         INTEGER PRIMARY KEY,
    market_id  INTEGER NOT NULL REFERENCES markets (id) ON DELETE CASCADE,
    item_key   TEXT    NOT NULL CHECK (length(item_key) > 2),
    product_id INTEGER REFERENCES products (id) ON DELETE CASCADE,
    ignored    BOOLEAN NOT NULL DEFAULT 0,
    UNIQUE (market_id, item_key),
    CHECK ((product_id IS NULL) = (ignored = 1))
);

-- +goose Down
DROP TABLE receipt_links;
ALTER TABLE prices DROP COLUMN receipt_id;
DROP TABLE receipts;
