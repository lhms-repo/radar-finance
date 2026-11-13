package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pressly/goose/v3"
)

// personalVersion gives every person a personal space and moves card payments there (Fase 9, RF-83).
const personalVersion = 22

// personalRebuild lists the steps of migration 22. The groups table is rebuilt with a kind and an owner, so
// every personal group can be named Pessoal while shared names stay unique.
var personalRebuild = []string{
	`CREATE TABLE groups_new (
    id              INTEGER  PRIMARY KEY,
    name            TEXT     NOT NULL COLLATE NOCASE CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    kind            TEXT     NOT NULL DEFAULT 'shared' CHECK (kind IN ('shared', 'personal')),
    owner_person_id INTEGER  UNIQUE REFERENCES persons (id),
    created_by      INTEGER  REFERENCES users (id),
    created_at      DATETIME NOT NULL,
    CHECK ((kind = 'personal') = (owner_person_id IS NOT NULL))
)`,
	`INSERT INTO groups_new (id, name, kind, created_by, created_at)
SELECT id, name, 'shared', created_by, created_at FROM groups`,
	`DROP TABLE groups`,
	`ALTER TABLE groups_new RENAME TO groups`,
	`CREATE UNIQUE INDEX groups_shared_name ON groups (name COLLATE NOCASE) WHERE kind = 'shared'`,

	// One personal group per person who can enter, with that person as its only member.
	`INSERT INTO groups (name, kind, owner_person_id, created_by, created_at)
SELECT 'Pessoal', 'personal', u.person_id, u.id, u.created_at FROM users u ORDER BY u.id`,
	`INSERT INTO group_members (group_id, person_id, role, weight_bp, joined_at)
SELECT g.id, g.owner_person_id, 'admin', 10000, g.created_at FROM groups g WHERE g.kind = 'personal'`,

	// The owner pays the whole invoice of her card, so payments leave the shared groups for her personal one,
	// and the account entry of each payment goes to the personal group of the account holder.
	`UPDATE invoice_payments SET group_id = (
    SELECT g.id FROM groups g JOIN cards c ON c.person_id = g.owner_person_id WHERE c.id = invoice_payments.card_id)`,
	`UPDATE entries SET group_id = (
    SELECT g.id FROM groups g JOIN accounts a ON a.person_id = g.owner_person_id WHERE a.id = entries.account_id)
WHERE kind = 'invoice_payment'`,
}

// personalMigration returns migration 22, run with foreign keys off since group_members cascades from groups.
func personalMigration() *goose.Migration {
	return goose.NewGoMigration(personalVersion, &goose.GoFunc{RunDB: rebuild("personal spaces", personalRebuild)}, &goose.GoFunc{RunDB: irreversiblePersonal})
}

// irreversiblePersonal refuses to undo migration 22: personal items would have no group to go back to.
func irreversiblePersonal(context.Context, *sql.DB) error {
	return errors.New("migration 22 is irreversible: restore a backup taken before it (docs/backup.md)")
}
