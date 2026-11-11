-- Fixed bills paid straight from an account, like rent, power and water (RF-35 to RF-37, RN-14).
--
-- As with incomes, the bill of each month is derived from the registration.
-- The expected amount holds until the real one is informed for that month
-- (RN-14). Paying writes an expense entry linked to the bill and the month,
-- so it counts as spending of the bill category; the amount paid becomes the
-- real amount of the month.

-- +goose Up
CREATE TABLE bills (
    id             INTEGER  PRIMARY KEY,
    name           TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    category_id    INTEGER  NOT NULL REFERENCES categories (id),
    account_id     INTEGER  NOT NULL REFERENCES accounts (id),
    due_day        INTEGER  NOT NULL CHECK (due_day BETWEEN 1 AND 31),
    expected_cents INTEGER  NOT NULL CHECK (expected_cents > 0),
    start_month    TEXT     NOT NULL,
    end_month      TEXT,
    created_by     INTEGER  NOT NULL REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    CHECK (end_month IS NULL OR end_month >= start_month)
);

-- Real amount informed for one month, kept apart from the expected one for comparison (RF-36).
CREATE TABLE bill_amounts (
    id           INTEGER PRIMARY KEY,
    bill_id      INTEGER NOT NULL REFERENCES bills (id) ON DELETE CASCADE,
    month        TEXT    NOT NULL,
    actual_cents INTEGER NOT NULL CHECK (actual_cents > 0),
    UNIQUE (bill_id, month)
);

ALTER TABLE entries ADD COLUMN bill_id INTEGER REFERENCES bills (id);
-- Month of the bill the entry pays, as YYYY-MM.
ALTER TABLE entries ADD COLUMN bill_month TEXT;

CREATE UNIQUE INDEX entries_bill_month_idx ON entries (bill_id, bill_month) WHERE bill_id IS NOT NULL;

-- +goose Down
DROP INDEX entries_bill_month_idx;
ALTER TABLE entries DROP COLUMN bill_month;
ALTER TABLE entries DROP COLUMN bill_id;
DROP TABLE bill_amounts;
DROP TABLE bills;
