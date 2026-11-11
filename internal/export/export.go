// Package export writes every business table as CSV inside a zip file (RNF-18).
package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// skippedTables are internal tables that are not user data.
var skippedTables = map[string]bool{
	"sessions":         true,
	"goose_db_version": true,
}

// skippedColumns are secrets that never leave the database.
var skippedColumns = map[string]bool{
	"password_hash": true,
}

// WriteZip writes one CSV per table from a single consistent read snapshot.
func WriteZip(ctx context.Context, conn *sql.DB, w io.Writer) error {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables, err := listTables(ctx, tx)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	for _, table := range tables {
		f, err := zw.Create(table + ".csv")
		if err != nil {
			return err
		}
		if err := writeTable(ctx, tx, table, f); err != nil {
			return fmt.Errorf("export %s: %w", table, err)
		}
	}
	return zw.Close()
}

// listTables returns the user tables in name order.
func listTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !skippedTables[name] {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// writeTable writes the header and every row of one table as CSV.
func writeTable(ctx context.Context, tx *sql.Tx, table string, w io.Writer) error {
	// The table name comes from sqlite_master, not from the user.
	rows, err := tx.QueryContext(ctx, `SELECT * FROM "`+table+`"`)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	keep := make([]int, 0, len(cols))
	header := make([]string, 0, len(cols))
	for i, c := range cols {
		if !skippedColumns[c] {
			keep = append(keep, i)
			header = append(header, c)
		}
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	record := make([]string, len(keep))
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for j, i := range keep {
			record[j] = formatValue(values[i])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

// formatValue renders a SQLite value as CSV text; money stays in integer cents.
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}
