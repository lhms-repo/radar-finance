-- Bank accounts and the entries that move their money (RF-04 to RF-08, RN-08).
--
-- entries holds every movement of a bank account: one off expenses, incomes,
-- balance adjustments, the two sides of a transfer and, later, invoice
-- payments. Amounts are signed cents: positive enters the account, negative
-- leaves it (RN-21). The balance only counts confirmed entries (RF-05, RN-13).
-- Card purchases live in the card tables; only the invoice payment touches an
-- account, so a purchase is never counted twice (RN-07).

-- +goose Up
CREATE TABLE accounts (
    id                    INTEGER  PRIMARY KEY,
    name                  TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    person_id             INTEGER  NOT NULL REFERENCES persons (id),
    initial_balance_cents INTEGER  NOT NULL,
    active                BOOLEAN  NOT NULL DEFAULT 1,
    created_by            INTEGER  NOT NULL REFERENCES users (id),
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL
);

CREATE TABLE transfers (
    id              INTEGER  PRIMARY KEY,
    from_account_id INTEGER  NOT NULL REFERENCES accounts (id),
    to_account_id   INTEGER  NOT NULL REFERENCES accounts (id),
    amount_cents    INTEGER  NOT NULL CHECK (amount_cents > 0),
    occurred_on     TEXT     NOT NULL,
    created_by      INTEGER  NOT NULL REFERENCES users (id),
    created_at      DATETIME NOT NULL,
    CHECK (from_account_id <> to_account_id)
);

CREATE TABLE entries (
    id           INTEGER  PRIMARY KEY,
    account_id   INTEGER  NOT NULL REFERENCES accounts (id),
    -- expense, income, adjustment or transfer; later phases add their own kinds.
    kind         TEXT     NOT NULL CHECK (length(kind) > 0),
    amount_cents INTEGER  NOT NULL CHECK (amount_cents <> 0),
    -- Business date in America/Sao_Paulo, as YYYY-MM-DD.
    occurred_on  TEXT     NOT NULL CHECK (occurred_on GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'),
    confirmed    BOOLEAN  NOT NULL DEFAULT 1,
    category_id  INTEGER  REFERENCES categories (id),
    description  TEXT     NOT NULL DEFAULT '' CHECK (length(description) <= 120),
    transfer_id  INTEGER  REFERENCES transfers (id) ON DELETE CASCADE,
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL,
    CHECK (kind <> 'expense' OR (amount_cents < 0 AND category_id IS NOT NULL)),
    CHECK (kind <> 'income' OR amount_cents > 0),
    CHECK ((kind = 'transfer') = (transfer_id IS NOT NULL))
);

CREATE INDEX entries_account_day_idx ON entries (account_id, occurred_on);
CREATE INDEX entries_day_idx ON entries (occurred_on);
CREATE INDEX entries_category_idx ON entries (category_id);
CREATE INDEX entries_transfer_idx ON entries (transfer_id);

-- +goose Down
DROP TABLE entries;
DROP TABLE transfers;
DROP TABLE accounts;
