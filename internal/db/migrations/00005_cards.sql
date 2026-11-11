-- Credit cards, purchases with installments, recurrences and invoice payments
-- (RF-14 to RF-23, RN-01 to RN-03, RN-07, RN-17).
--
-- The invoice month is the due month (RN-01). Purchases and credits are split
-- into installments, one per invoice (RN-03); credits are negative. The
-- charges of a recurrence are derived from it and from its price history, so
-- a new price applies from a given invoice on without touching closed ones.
-- Paying an invoice writes an invoice_payment entry in the chosen account,
-- which is not spending (RN-07); deleting that entry deletes the payment.

-- +goose Up
CREATE TABLE cards (
    id          INTEGER  PRIMARY KEY,
    name        TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    person_id   INTEGER  NOT NULL REFERENCES persons (id),
    limit_cents INTEGER  NOT NULL CHECK (limit_cents >= 0),
    closing_day INTEGER  NOT NULL CHECK (closing_day BETWEEN 1 AND 31),
    due_day     INTEGER  NOT NULL CHECK (due_day BETWEEN 1 AND 31),
    last_digits TEXT     CHECK (last_digits IS NULL OR last_digits GLOB '[0-9][0-9][0-9][0-9]'),
    color       TEXT     NOT NULL,
    finish      TEXT     NOT NULL,
    active      BOOLEAN  NOT NULL DEFAULT 1,
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);

CREATE TABLE purchases (
    id                INTEGER  PRIMARY KEY,
    card_id           INTEGER  NOT NULL REFERENCES cards (id),
    -- purchase or credit (refunds and other credits on the invoice, RF-22).
    kind              TEXT     NOT NULL CHECK (kind IN ('purchase', 'credit')),
    -- Kept only as information; the invoice month is what counts (RN-01).
    purchased_on      TEXT     NOT NULL,
    merchant          TEXT     NOT NULL CHECK (length(trim(merchant)) > 0 AND length(merchant) <= 80),
    description       TEXT     NOT NULL DEFAULT '' CHECK (length(description) <= 120),
    category_id       INTEGER  REFERENCES categories (id),
    buyer_person_id   INTEGER  REFERENCES persons (id),
    total_cents       INTEGER  NOT NULL CHECK (total_cents > 0),
    installment_count INTEGER  NOT NULL CHECK (installment_count BETWEEN 1 AND 48),
    first_invoice     TEXT     NOT NULL,
    created_by        INTEGER  NOT NULL REFERENCES users (id),
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL,
    CHECK (kind <> 'purchase' OR category_id IS NOT NULL),
    CHECK (kind <> 'credit' OR installment_count = 1)
);

CREATE INDEX purchases_card_idx ON purchases (card_id);
CREATE INDEX purchases_category_idx ON purchases (category_id);

CREATE TABLE installments (
    id            INTEGER PRIMARY KEY,
    purchase_id   INTEGER NOT NULL REFERENCES purchases (id) ON DELETE CASCADE,
    card_id       INTEGER NOT NULL REFERENCES cards (id),
    invoice_month TEXT    NOT NULL,
    number        INTEGER NOT NULL CHECK (number >= 1),
    -- Signed: purchases are positive, credits negative.
    amount_cents  INTEGER NOT NULL CHECK (amount_cents <> 0),
    UNIQUE (purchase_id, number)
);

CREATE INDEX installments_card_month_idx ON installments (card_id, invoice_month);

CREATE TABLE recurrences (
    id          INTEGER  PRIMARY KEY,
    card_id     INTEGER  NOT NULL REFERENCES cards (id),
    description TEXT     NOT NULL CHECK (length(trim(description)) > 0 AND length(description) <= 80),
    category_id INTEGER  NOT NULL REFERENCES categories (id),
    start_month TEXT     NOT NULL,
    end_month   TEXT,
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    CHECK (end_month IS NULL OR end_month >= start_month)
);

CREATE TABLE recurrence_prices (
    id            INTEGER PRIMARY KEY,
    recurrence_id INTEGER NOT NULL REFERENCES recurrences (id) ON DELETE CASCADE,
    from_month    TEXT    NOT NULL,
    amount_cents  INTEGER NOT NULL CHECK (amount_cents > 0),
    UNIQUE (recurrence_id, from_month)
);

CREATE TABLE invoice_payments (
    id            INTEGER  PRIMARY KEY,
    card_id       INTEGER  NOT NULL REFERENCES cards (id),
    invoice_month TEXT     NOT NULL,
    entry_id      INTEGER  NOT NULL UNIQUE REFERENCES entries (id) ON DELETE CASCADE,
    amount_cents  INTEGER  NOT NULL CHECK (amount_cents > 0),
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    UNIQUE (card_id, invoice_month)
);

-- +goose Down
DROP TABLE invoice_payments;
DROP TABLE recurrence_prices;
DROP TABLE recurrences;
DROP TABLE installments;
DROP TABLE purchases;
DROP TABLE cards;
