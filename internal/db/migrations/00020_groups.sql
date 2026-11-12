-- Groups of people, each person in one or more, with a PIN to enter (Fase 8, RF-01, RF-78 to RF-82, RN-27).
--
-- The couple of a running Radar becomes the first group, "Casa", with both people as administrators and half
-- the weight each; every row of the tables that belong to a group goes to it. A new installation, with no
-- user yet, gets no group: the first one comes from the terminal (RF-80).
--
-- group_id stays nullable here so the code of the earlier phases keeps writing while each package learns to
-- scope by group; the services require it from then on. Migration 21, in Go, rebuilds the tables whose
-- unique names become unique per group, which needs foreign keys off and cannot run in this transaction.

-- +goose Up
CREATE TABLE groups (
    id         INTEGER  PRIMARY KEY,
    name       TEXT     NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    created_by INTEGER  REFERENCES users (id),
    created_at DATETIME NOT NULL
);

-- Weight in basis points, used by the settlement between members (RF-79, RN-28).
CREATE TABLE group_members (
    group_id  INTEGER  NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    person_id INTEGER  NOT NULL REFERENCES persons (id),
    role      TEXT     NOT NULL CHECK (role IN ('admin', 'member')),
    weight_bp INTEGER  NOT NULL CHECK (weight_bp BETWEEN 1 AND 10000),
    joined_at DATETIME NOT NULL,
    PRIMARY KEY (group_id, person_id)
);

CREATE INDEX group_members_person_idx ON group_members (person_id);

-- The PIN is kept only as a hash; five wrong ones in a row lock the person out for 15 minutes (RN-27).
ALTER TABLE users ADD COLUMN pin_hash TEXT;
ALTER TABLE users ADD COLUMN pin_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN pin_locked_until DATETIME;

ALTER TABLE entries ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE purchases ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE recurrences ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE bills ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE financings ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE incomes ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE alelo_purchases ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE savings_boxes ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE shopping_lists ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE products ADD COLUMN group_id INTEGER REFERENCES groups (id);
ALTER TABLE imports ADD COLUMN group_id INTEGER REFERENCES groups (id);

INSERT INTO groups (id, name, created_at)
SELECT 1, 'Casa', CURRENT_TIMESTAMP
WHERE EXISTS (SELECT 1 FROM users);

-- Equal weights; the basis points that do not divide evenly go to the first person.
INSERT INTO group_members (group_id, person_id, role, weight_bp, joined_at)
SELECT 1, u.person_id, 'admin',
       10000 / (SELECT count(*) FROM users) + CASE WHEN u.position = 1 THEN 10000 % (SELECT count(*) FROM users) ELSE 0 END,
       CURRENT_TIMESTAMP
FROM (SELECT person_id, row_number() OVER (ORDER BY id) AS position FROM users) AS u
WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);

UPDATE entries SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE purchases SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE recurrences SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE bills SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE financings SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE incomes SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE alelo_purchases SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE savings_boxes SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE shopping_lists SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE products SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);
UPDATE imports SET group_id = 1 WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);

-- Warnings are read per group: the same card or category may warn in two groups (RF-64).
UPDATE notification_reads SET key = 'g1:' || key WHERE EXISTS (SELECT 1 FROM groups WHERE id = 1);

CREATE INDEX entries_group_idx ON entries (group_id);
CREATE INDEX purchases_group_idx ON purchases (group_id);
CREATE INDEX recurrences_group_idx ON recurrences (group_id);
CREATE INDEX bills_group_idx ON bills (group_id);
CREATE INDEX financings_group_idx ON financings (group_id);
CREATE INDEX incomes_group_idx ON incomes (group_id);
CREATE INDEX alelo_purchases_group_idx ON alelo_purchases (group_id);
CREATE INDEX savings_boxes_group_idx ON savings_boxes (group_id);
CREATE INDEX shopping_lists_group_idx ON shopping_lists (group_id);
CREATE INDEX imports_group_idx ON imports (group_id);

-- A product is the same brand and size inside one group; another group keeps its own catalog (RF-85).
DROP INDEX products_identity;
CREATE UNIQUE INDEX products_identity ON products (group_id, name COLLATE NOCASE, brand COLLATE NOCASE, by_weight, size_milli, unit);

-- +goose Down
-- Irreversible: dropping group_id would need a rebuild of every table above, and a group created after this
-- migration has no place to go back to. Restore a backup taken before it instead (docs/backup.md).
SELECT 1;
