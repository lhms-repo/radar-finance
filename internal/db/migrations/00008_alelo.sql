-- Alelo meal cards: monthly credit, free text purchases and the balance carried
-- to the next month (RF-31 to RF-34, RN-15).
--
-- Balance of a month = balance of the previous month + confirmed credit -
-- purchases of the month (RN-15), starting from the balance on the day the
-- card was registered. Purchases count as spending in Alimentação, Compras
-- para casa; that subcategory gets a system key so a rename does not break
-- the link, and it cannot be deleted.

-- +goose Up
ALTER TABLE categories ADD COLUMN system_key TEXT;
CREATE UNIQUE INDEX categories_system_key_idx ON categories (system_key) WHERE system_key IS NOT NULL;
UPDATE categories SET system_key = 'groceries'
WHERE name = 'Compras para casa' AND parent_id = (SELECT id FROM categories WHERE name = 'Alimentação' AND parent_id IS NULL);

CREATE TABLE alelo_cards (
    id                    INTEGER  PRIMARY KEY,
    name                  TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    person_id             INTEGER  NOT NULL REFERENCES persons (id),
    monthly_credit_cents  INTEGER  NOT NULL CHECK (monthly_credit_cents > 0),
    credit_day            INTEGER  NOT NULL CHECK (credit_day BETWEEN 1 AND 31),
    initial_balance_cents INTEGER  NOT NULL CHECK (initial_balance_cents >= 0),
    -- First month with a planned credit, the month of the registration.
    start_month           TEXT     NOT NULL,
    active                BOOLEAN  NOT NULL DEFAULT 1,
    created_by            INTEGER  NOT NULL REFERENCES users (id),
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL
);

CREATE TABLE alelo_credits (
    id            INTEGER  PRIMARY KEY,
    alelo_id      INTEGER  NOT NULL REFERENCES alelo_cards (id),
    month         TEXT     NOT NULL,
    amount_cents  INTEGER  NOT NULL CHECK (amount_cents > 0),
    credited_on   TEXT     NOT NULL,
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    UNIQUE (alelo_id, month)
);

CREATE TABLE alelo_purchases (
    id           INTEGER  PRIMARY KEY,
    alelo_id     INTEGER  NOT NULL REFERENCES alelo_cards (id),
    purchased_on TEXT     NOT NULL,
    description  TEXT     NOT NULL CHECK (length(trim(description)) > 0 AND length(description) <= 120),
    amount_cents INTEGER  NOT NULL CHECK (amount_cents > 0),
    -- Always the groceries subcategory (RN-15), kept for the spending reports.
    category_id  INTEGER  NOT NULL REFERENCES categories (id),
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL
);

CREATE INDEX alelo_purchases_card_day_idx ON alelo_purchases (alelo_id, purchased_on);

-- +goose Down
DROP TABLE alelo_purchases;
DROP TABLE alelo_credits;
DROP TABLE alelo_cards;
DROP INDEX categories_system_key_idx;
ALTER TABLE categories DROP COLUMN system_key;
