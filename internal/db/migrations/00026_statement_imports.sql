-- Import of the account statement CSV of Santander (RF-86 to RF-90, RN-32).
--
-- Each import is kept with its account, so it can be undone whole, and every entry it created points to it.
-- The default subcategories of statement lines get system keys, and Financeiro gains Investimentos for the
-- money sent to applications (RF-87).

-- +goose Up
CREATE TABLE statement_imports (
    id            INTEGER  PRIMARY KEY,
    group_id      INTEGER  NOT NULL REFERENCES groups (id),
    account_id    INTEGER  NOT NULL REFERENCES accounts (id),
    file_name     TEXT     NOT NULL DEFAULT '' CHECK (length(file_name) <= 200),
    line_count    INTEGER  NOT NULL,
    closing_cents INTEGER  NOT NULL,
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL
);

CREATE INDEX statement_imports_group_idx ON statement_imports (group_id);

ALTER TABLE entries ADD COLUMN statement_import_id INTEGER REFERENCES statement_imports (id);

CREATE INDEX entries_statement_import_idx ON entries (statement_import_id) WHERE statement_import_id IS NOT NULL;

INSERT INTO categories (parent_id, name, sort_order, created_at, updated_at)
SELECT p.id, 'Investimentos', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM categories p
WHERE p.name = 'Financeiro' AND p.parent_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM categories c WHERE c.parent_id = p.id AND c.name = 'Investimentos');

UPDATE categories SET system_key = 'bank_fees'
WHERE system_key IS NULL AND name = 'Tarifas bancárias' AND parent_id = (SELECT id FROM categories WHERE name = 'Financeiro' AND parent_id IS NULL);
UPDATE categories SET system_key = 'taxes'
WHERE system_key IS NULL AND name = 'Impostos' AND parent_id = (SELECT id FROM categories WHERE name = 'Financeiro' AND parent_id IS NULL);
UPDATE categories SET system_key = 'interest'
WHERE system_key IS NULL AND name = 'Juros e multas' AND parent_id = (SELECT id FROM categories WHERE name = 'Financeiro' AND parent_id IS NULL);
UPDATE categories SET system_key = 'investments'
WHERE system_key IS NULL AND name = 'Investimentos' AND parent_id = (SELECT id FROM categories WHERE name = 'Financeiro' AND parent_id IS NULL);

-- +goose Down
UPDATE categories SET system_key = NULL WHERE system_key IN ('bank_fees', 'taxes', 'interest', 'investments');
DROP INDEX entries_statement_import_idx;
ALTER TABLE entries DROP COLUMN statement_import_id;
DROP TABLE statement_imports;
