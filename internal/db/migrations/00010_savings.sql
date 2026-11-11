-- Savings boxes with a target and a monthly percentage of the income of the
-- month (RF-43 to RF-49, RN-09 to RN-12).
--
-- The balance is the starting balance plus deposits minus withdrawals. A
-- deposit takes money out of a bank account and a withdrawal puts it back;
-- both are account entries that are neither spending nor income (RN-08).
-- Deleting the entry deletes the move. The percentage is in basis points,
-- 1250 meaning 12,5%, so no floating point is ever used.

-- +goose Up
CREATE TABLE savings_boxes (
    id                    INTEGER  PRIMARY KEY,
    name                  TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    target_cents          INTEGER  NOT NULL CHECK (target_cents > 0),
    percent_bp            INTEGER  NOT NULL CHECK (percent_bp BETWEEN 0 AND 10000),
    initial_balance_cents INTEGER  NOT NULL CHECK (initial_balance_cents >= 0),
    created_by            INTEGER  NOT NULL REFERENCES users (id),
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL
);

CREATE TABLE savings_moves (
    id           INTEGER  PRIMARY KEY,
    box_id       INTEGER  NOT NULL REFERENCES savings_boxes (id),
    kind         TEXT     NOT NULL CHECK (kind IN ('deposit', 'withdrawal')),
    -- Month the deposit belongs to, for the suggestion of that month.
    month        TEXT     NOT NULL,
    amount_cents INTEGER  NOT NULL CHECK (amount_cents > 0),
    reason       TEXT     NOT NULL DEFAULT '' CHECK (length(reason) <= 120),
    entry_id     INTEGER  NOT NULL UNIQUE REFERENCES entries (id) ON DELETE CASCADE,
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL
);

CREATE INDEX savings_moves_box_idx ON savings_moves (box_id, month);

-- +goose Down
DROP TABLE savings_moves;
DROP TABLE savings_boxes;
