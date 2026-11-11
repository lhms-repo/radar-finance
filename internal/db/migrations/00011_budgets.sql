-- Monthly budget of each top level category, the same every month (RF-51).
-- The spending it is compared with sums every subcategory (RF-52).

-- +goose Up
CREATE TABLE budgets (
    id           INTEGER  PRIMARY KEY,
    category_id  INTEGER  NOT NULL UNIQUE REFERENCES categories (id) ON DELETE CASCADE,
    amount_cents INTEGER  NOT NULL CHECK (amount_cents > 0),
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    updated_at   DATETIME NOT NULL
);

-- +goose Down
DROP TABLE budgets;
