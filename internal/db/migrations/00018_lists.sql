-- Shopping lists shared by the couple, with products, quantities and the
-- picked mark (RF-71). Quantities are thousandths: whole units for packaged
-- products and kg with decimals for products sold by weight (RF-66).

-- +goose Up
CREATE TABLE shopping_lists (
    id          INTEGER  PRIMARY KEY,
    name        TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    status      TEXT     NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL,
    finished_at DATETIME
);

CREATE TABLE list_items (
    id             INTEGER  PRIMARY KEY,
    list_id        INTEGER  NOT NULL REFERENCES shopping_lists (id) ON DELETE CASCADE,
    product_id     INTEGER  NOT NULL REFERENCES products (id),
    quantity_milli INTEGER  NOT NULL CHECK (quantity_milli > 0),
    picked         BOOLEAN  NOT NULL DEFAULT 0,
    picked_by      INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    UNIQUE (list_id, product_id)
);

-- +goose Down
DROP TABLE list_items;
DROP TABLE shopping_lists;
