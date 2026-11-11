-- Financings such as the house, the lot, the car and solar power (RF-39 to RF-42, RN-16).
--
-- The registration keeps the state on the day it was made: installment value,
-- total and paid installments, outstanding balance and the month of the next
-- installment. The installment of each month is derived from it and shows up
-- as a bill to pay (RF-40). Payments and manual corrections are events, each
-- with the outstanding balance after it; the current balance is the one of
-- the latest event (RN-16, RF-42). Deleting a payment entry deletes its event,
-- so the balance goes back by itself.

-- +goose Up
CREATE TABLE financings (
    id                   INTEGER  PRIMARY KEY,
    name                 TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    category_id          INTEGER  NOT NULL REFERENCES categories (id),
    account_id           INTEGER  NOT NULL REFERENCES accounts (id),
    due_day              INTEGER  NOT NULL CHECK (due_day BETWEEN 1 AND 31),
    installment_cents    INTEGER  NOT NULL CHECK (installment_cents > 0),
    total_installments   INTEGER  NOT NULL CHECK (total_installments BETWEEN 1 AND 600),
    initial_paid         INTEGER  NOT NULL CHECK (initial_paid >= 0 AND initial_paid <= total_installments),
    initial_balance_cents INTEGER NOT NULL CHECK (initial_balance_cents >= 0),
    -- Month of installment number initial_paid + 1.
    start_month          TEXT     NOT NULL,
    created_by           INTEGER  NOT NULL REFERENCES users (id),
    created_at           DATETIME NOT NULL,
    updated_at           DATETIME NOT NULL
);

CREATE TABLE financing_events (
    id                  INTEGER  PRIMARY KEY,
    financing_id        INTEGER  NOT NULL REFERENCES financings (id) ON DELETE CASCADE,
    kind                TEXT     NOT NULL CHECK (kind IN ('payment', 'correction')),
    -- Payment only: month of the installment and the account entry that paid it.
    month               TEXT,
    number              INTEGER,
    entry_id            INTEGER  UNIQUE REFERENCES entries (id) ON DELETE CASCADE,
    balance_after_cents INTEGER  NOT NULL CHECK (balance_after_cents >= 0),
    created_by          INTEGER  NOT NULL REFERENCES users (id),
    created_at          DATETIME NOT NULL,
    CHECK ((kind = 'payment') = (entry_id IS NOT NULL AND month IS NOT NULL AND number IS NOT NULL))
);

CREATE INDEX financing_events_financing_idx ON financing_events (financing_id, id);
CREATE UNIQUE INDEX financing_events_month_idx ON financing_events (financing_id, month) WHERE kind = 'payment';

-- +goose Down
DROP TABLE financing_events;
DROP TABLE financings;
