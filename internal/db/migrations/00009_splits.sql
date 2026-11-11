-- Purchases split with other people, named only, and the share of each one in
-- each installment (RF-24 to RF-27, RN-04, RN-05).
--
-- A split keeps the whole share of a person in a purchase; the share is spread
-- over the installments like the purchase itself, with the leftover cents on
-- the first one (RN-03). Marking a share as received writes an entry in the
-- chosen account (RN-06); deleting that entry unmarks it.

-- +goose Up
CREATE TABLE parties (
    id         INTEGER  PRIMARY KEY,
    name       TEXT     NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    created_by INTEGER  NOT NULL REFERENCES users (id),
    created_at DATETIME NOT NULL
);

CREATE TABLE splits (
    id          INTEGER PRIMARY KEY,
    purchase_id INTEGER NOT NULL REFERENCES purchases (id) ON DELETE CASCADE,
    party_id    INTEGER NOT NULL REFERENCES parties (id),
    share_cents INTEGER NOT NULL CHECK (share_cents > 0),
    UNIQUE (purchase_id, party_id)
);

CREATE TABLE split_installments (
    id            INTEGER PRIMARY KEY,
    split_id      INTEGER NOT NULL REFERENCES splits (id) ON DELETE CASCADE,
    number        INTEGER NOT NULL CHECK (number >= 1),
    card_id       INTEGER NOT NULL REFERENCES cards (id),
    invoice_month TEXT    NOT NULL,
    amount_cents  INTEGER NOT NULL CHECK (amount_cents > 0),
    -- Entry that received this share in an account; NULL while pending.
    entry_id      INTEGER UNIQUE REFERENCES entries (id) ON DELETE SET NULL,
    UNIQUE (split_id, number)
);

CREATE INDEX split_installments_card_month_idx ON split_installments (card_id, invoice_month);

-- +goose Down
DROP TABLE split_installments;
DROP TABLE splits;
DROP TABLE parties;
