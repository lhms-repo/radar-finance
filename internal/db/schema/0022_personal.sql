-- Read only by sqlc, after the migrations: it mirrors the columns that the Go migration 22
-- (internal/db/migrate_personal.go) adds when it rebuilds the groups table. goose never runs this file.
ALTER TABLE groups ADD COLUMN kind TEXT NOT NULL DEFAULT 'shared' CHECK (kind IN ('shared', 'personal'));
ALTER TABLE groups ADD COLUMN owner_person_id INTEGER UNIQUE REFERENCES persons (id);
