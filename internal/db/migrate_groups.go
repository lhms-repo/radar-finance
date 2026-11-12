package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
)

// groupRebuildVersion rebuilds the tables whose unique names become unique per group (Fase 8, RF-85).
const groupRebuildVersion = 21

// groupRebuild lists the steps of migration 21: each table is created anew with group_id, filled from the
// old one, which is then dropped and replaced, as the SQLite guide for schema changes describes.
var groupRebuild = []string{
	// Names of other people in splits are unique inside each group (RF-27).
	`CREATE TABLE parties_new (
    id         INTEGER  PRIMARY KEY,
    group_id   INTEGER  REFERENCES groups (id),
    name       TEXT     NOT NULL COLLATE NOCASE CHECK (length(trim(name)) > 0 AND length(name) <= 40),
    created_by INTEGER  NOT NULL REFERENCES users (id),
    created_at DATETIME NOT NULL,
    UNIQUE (group_id, name)
)`,
	`INSERT INTO parties_new (id, group_id, name, created_by, created_at)
SELECT id, (SELECT id FROM groups WHERE id = 1), name, created_by, created_at FROM parties`,
	`DROP TABLE parties`,
	`ALTER TABLE parties_new RENAME TO parties`,

	// One budget per top level category in each group (RF-51).
	`CREATE TABLE budgets_new (
    id           INTEGER  PRIMARY KEY,
    group_id     INTEGER  REFERENCES groups (id),
    category_id  INTEGER  NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    amount_cents INTEGER  NOT NULL CHECK (amount_cents > 0),
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    updated_at   DATETIME NOT NULL,
    UNIQUE (group_id, category_id)
)`,
	`INSERT INTO budgets_new (id, group_id, category_id, amount_cents, created_by, updated_at)
SELECT id, (SELECT id FROM groups WHERE id = 1), category_id, amount_cents, created_by, updated_at FROM budgets`,
	`DROP TABLE budgets`,
	`ALTER TABLE budgets_new RENAME TO budgets`,

	// Each group has its own markets; the CNPJ and the name in a neighborhood repeat only across groups (RF-85).
	`CREATE TABLE markets_new (
    id           INTEGER  PRIMARY KEY,
    group_id     INTEGER  REFERENCES groups (id),
    name         TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    neighborhood TEXT     NOT NULL DEFAULT '' CHECK (length(neighborhood) <= 60),
    cnpj         TEXT     CHECK (cnpj IS NULL OR length(cnpj) = 14),
    active       BOOLEAN  NOT NULL DEFAULT 1,
    created_by   INTEGER  NOT NULL REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (group_id, cnpj)
)`,
	`INSERT INTO markets_new (id, group_id, name, neighborhood, cnpj, active, created_by, created_at)
SELECT id, (SELECT id FROM groups WHERE id = 1), name, neighborhood, cnpj, active, created_by, created_at FROM markets`,
	`DROP TABLE markets`,
	`ALTER TABLE markets_new RENAME TO markets`,
	`CREATE UNIQUE INDEX markets_name_neighborhood ON markets (group_id, name COLLATE NOCASE, neighborhood COLLATE NOCASE)`,

	// A receipt is read once per market, and markets belong to a group, so two groups may read the same one.
	`CREATE TABLE receipts_new (
    id          INTEGER  PRIMARY KEY,
    market_id   INTEGER  NOT NULL REFERENCES markets (id),
    access_key  TEXT     NOT NULL CHECK (length(access_key) = 44),
    issued_on   TEXT     NOT NULL,
    total_cents INTEGER  NOT NULL CHECK (total_cents >= 0),
    item_count  INTEGER  NOT NULL CHECK (item_count >= 0),
    created_by  INTEGER  NOT NULL REFERENCES users (id),
    created_at  DATETIME NOT NULL,
    UNIQUE (market_id, access_key)
)`,
	`INSERT INTO receipts_new (id, market_id, access_key, issued_on, total_cents, item_count, created_by, created_at)
SELECT id, market_id, access_key, issued_on, total_cents, item_count, created_by, created_at FROM receipts`,
	`DROP TABLE receipts`,
	`ALTER TABLE receipts_new RENAME TO receipts`,

	// One row of comparison settings per group (RF-74). The old row keeps its id, so the code that still
	// reads id 1 works until it learns the group.
	`CREATE TABLE market_settings_new (
    id                INTEGER PRIMARY KEY,
    group_id          INTEGER UNIQUE REFERENCES groups (id) ON DELETE CASCADE,
    max_markets       INTEGER NOT NULL CHECK (max_markets BETWEEN 1 AND 5),
    min_savings_cents INTEGER NOT NULL CHECK (min_savings_cents >= 0),
    stale_days        INTEGER NOT NULL CHECK (stale_days BETWEEN 1 AND 365)
)`,
	`INSERT INTO market_settings_new (id, group_id, max_markets, min_savings_cents, stale_days)
SELECT id, (SELECT id FROM groups WHERE id = 1), max_markets, min_savings_cents, stale_days FROM market_settings`,
	`DROP TABLE market_settings`,
	`ALTER TABLE market_settings_new RENAME TO market_settings`,

	// A card shared by a person in two groups has one invoice per group, each paid in its group (RF-21, RF-82).
	`CREATE TABLE invoice_payments_new (
    id            INTEGER  PRIMARY KEY,
    group_id      INTEGER  REFERENCES groups (id),
    card_id       INTEGER  NOT NULL REFERENCES cards (id),
    invoice_month TEXT     NOT NULL,
    entry_id      INTEGER  NOT NULL UNIQUE REFERENCES entries (id) ON DELETE CASCADE,
    amount_cents  INTEGER  NOT NULL CHECK (amount_cents > 0),
    created_by    INTEGER  NOT NULL REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    UNIQUE (group_id, card_id, invoice_month)
)`,
	`INSERT INTO invoice_payments_new (id, group_id, card_id, invoice_month, entry_id, amount_cents, created_by, created_at)
SELECT id, (SELECT id FROM groups WHERE id = 1), card_id, invoice_month, entry_id, amount_cents, created_by, created_at FROM invoice_payments`,
	`DROP TABLE invoice_payments`,
	`ALTER TABLE invoice_payments_new RENAME TO invoice_payments`,
}

// groupMigrations returns the Go migrations of the groups change, registered beside the SQL files.
func groupMigrations() []*goose.Migration {
	return []*goose.Migration{goose.NewGoMigration(groupRebuildVersion, &goose.GoFunc{RunDB: rebuildForGroups}, nil)}
}

// rebuildForGroups runs the rebuild on one dedicated connection with foreign keys off, so dropping a parent
// table neither fails nor cascades into its children, and checks every reference before committing.
func rebuildForGroups(ctx context.Context, pool *sql.DB) (err error) {
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// The pragma only works outside a transaction and only on this connection.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() {
		if _, onErr := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); onErr != nil {
			err = errors.Join(err, fmt.Errorf("turn foreign keys back on: %w", onErr))
		}
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, stmt := range groupRebuild {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("rebuild for groups: %w\n%s", err, stmt)
		}
	}
	broken, err := hasBrokenReferences(ctx, tx)
	if err != nil {
		return err
	}
	if broken {
		return errors.New("rebuild for groups: foreign key check found broken references")
	}
	return tx.Commit()
}

// hasBrokenReferences tells whether any foreign key of the database points to a missing row.
func hasBrokenReferences(ctx context.Context, tx *sql.Tx) (bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	broken := rows.Next()
	return broken, rows.Err()
}
