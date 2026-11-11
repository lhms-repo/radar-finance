-- Recurring and eventual incomes (RF-09 to RF-13, RN-13).
--
-- An income does not store one row per month: the planned occurrences of a
-- month are derived from it. Confirming the receipt writes a confirmed entry
-- linked to the income and to the occurrence month, and only then the account
-- balance changes (RN-13). A one off income is a plain income entry.

-- +goose Up
CREATE TABLE incomes (
    id           INTEGER  PRIMARY KEY,
    kind         TEXT     NOT NULL CHECK (kind IN ('recurring', 'eventual')),
    name         TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    person_id    INTEGER  NOT NULL REFERENCES persons (id),
    account_id   INTEGER  NOT NULL REFERENCES accounts (id),
    amount_cents INTEGER  NOT NULL CHECK (amount_cents > 0),
    -- Recurring: day of the month (clamped to short months), first month and optional end date.
    day          INTEGER  CHECK (day BETWEEN 1 AND 31),
    start_month  TEXT,
    end_on       TEXT,
    -- Eventual: the estimated date.
    expected_on  TEXT,
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL,
    CHECK (kind <> 'recurring' OR (day IS NOT NULL AND start_month IS NOT NULL AND expected_on IS NULL)),
    CHECK (kind <> 'eventual' OR (expected_on IS NOT NULL AND day IS NULL AND start_month IS NULL AND end_on IS NULL))
);

ALTER TABLE entries ADD COLUMN income_id INTEGER REFERENCES incomes (id);
-- Month of the occurrence the entry confirms, as YYYY-MM.
ALTER TABLE entries ADD COLUMN income_month TEXT;

CREATE UNIQUE INDEX entries_income_month_idx ON entries (income_id, income_month) WHERE income_id IS NOT NULL;

-- +goose Down
DROP INDEX entries_income_month_idx;
ALTER TABLE entries DROP COLUMN income_month;
ALTER TABLE entries DROP COLUMN income_id;
DROP TABLE incomes;
