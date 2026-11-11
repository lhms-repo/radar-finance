-- Import of the Nubank invoice CSV: alias rules, import history and the link
-- from each imported purchase to its import (RF-53 to RF-57, RN-19, RN-20).
--
-- Alias rules match the original merchant of the CSV, normalized to lower case
-- with single spaces; with two rules for the same text the most recent wins
-- (RN-19). "Sem categoria definida" gets a system key: it is the category of
-- lines without a rule.

-- +goose Up
UPDATE categories SET system_key = 'uncategorized'
WHERE name = 'Sem categoria definida' AND parent_id = (SELECT id FROM categories WHERE name = 'Outros' AND parent_id IS NULL);

CREATE TABLE alias_rules (
    id            INTEGER  PRIMARY KEY,
    original      TEXT     NOT NULL CHECK (length(original) > 0 AND length(original) <= 120),
    friendly_name TEXT     NOT NULL CHECK (length(trim(friendly_name)) > 0 AND length(friendly_name) <= 80),
    category_id   INTEGER  REFERENCES categories (id),
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL
);

CREATE INDEX alias_rules_original_idx ON alias_rules (original, id);

CREATE TABLE imports (
    id            INTEGER  PRIMARY KEY,
    card_id       INTEGER  NOT NULL REFERENCES cards (id),
    invoice_month TEXT     NOT NULL,
    file_name     TEXT     NOT NULL DEFAULT '' CHECK (length(file_name) <= 200),
    line_count    INTEGER  NOT NULL,
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL
);

ALTER TABLE purchases ADD COLUMN import_id INTEGER REFERENCES imports (id);

-- +goose Down
ALTER TABLE purchases DROP COLUMN import_id;
DROP TABLE imports;
DROP TABLE alias_rules;
UPDATE categories SET system_key = NULL WHERE system_key = 'uncategorized';
