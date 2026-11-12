// Package export writes the data of the active group as CSV inside a zip file (RNF-18, RF-82).
package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/scope"
)

// skippedTables are internal tables that are not user data.
var skippedTables = map[string]bool{
	"sessions":         true,
	"goose_db_version": true,
}

// members selects the people of the active group; accounts, cards and Alelo cards belong to a person and are
// seen by the group while their holder is a member (RF-78).
const members = `SELECT person_id FROM group_members WHERE group_id = @g`

// ofMembers keeps the rows held by a member of the active group.
const ofMembers = `person_id IN (` + members + `)`

// rules says which rows of each table the active group exports: its own rows, the rows of its parents'
// children, the people who are members and the personal data of the user asking. A table without a rule is
// refused, so a new table never leaves the house by accident.
var rules = map[string]string{
	// Rows with the group on them.
	"entries":         `SELECT * FROM entries WHERE group_id = @g`,
	"purchases":       `SELECT * FROM purchases WHERE group_id = @g`,
	"recurrences":     `SELECT * FROM recurrences WHERE group_id = @g`,
	"bills":           `SELECT * FROM bills WHERE group_id = @g`,
	"financings":      `SELECT * FROM financings WHERE group_id = @g`,
	"incomes":         `SELECT * FROM incomes WHERE group_id = @g`,
	"alelo_purchases": `SELECT * FROM alelo_purchases WHERE group_id = @g`,
	"savings_boxes":   `SELECT * FROM savings_boxes WHERE group_id = @g`,
	"shopping_lists":  `SELECT * FROM shopping_lists WHERE group_id = @g`,
	"products":        `SELECT * FROM products WHERE group_id = @g`,
	"parties":         `SELECT * FROM parties WHERE group_id = @g`,
	"budgets":         `SELECT * FROM budgets WHERE group_id = @g`,
	"markets":         `SELECT * FROM markets WHERE group_id = @g`,
	"market_settings": `SELECT * FROM market_settings WHERE group_id = @g`,
	"groups":          `SELECT * FROM groups WHERE id = @g`,
	"group_members":   `SELECT * FROM group_members WHERE group_id = @g`,

	// Rows of the people of the group.
	"persons":     `SELECT * FROM persons WHERE id IN (` + members + `)`,
	"accounts":    `SELECT * FROM accounts WHERE ` + ofMembers,
	"cards":       `SELECT * FROM cards WHERE ` + ofMembers,
	"alelo_cards": `SELECT * FROM alelo_cards WHERE ` + ofMembers,

	// Rows that follow a parent of the group.
	"installments":       `SELECT * FROM installments WHERE purchase_id IN (SELECT id FROM purchases WHERE group_id = @g)`,
	"splits":             `SELECT * FROM splits WHERE purchase_id IN (SELECT id FROM purchases WHERE group_id = @g)`,
	"split_installments": `SELECT * FROM split_installments WHERE split_id IN (SELECT s.id FROM splits s JOIN purchases p ON p.id = s.purchase_id WHERE p.group_id = @g)`,
	"recurrence_prices":  `SELECT * FROM recurrence_prices WHERE recurrence_id IN (SELECT id FROM recurrences WHERE group_id = @g)`,
	"invoice_payments":   `SELECT * FROM invoice_payments WHERE entry_id IN (SELECT id FROM entries WHERE group_id = @g)`,
	"transfers":          `SELECT * FROM transfers WHERE id IN (SELECT transfer_id FROM entries WHERE group_id = @g AND transfer_id IS NOT NULL)`,
	"bill_amounts":       `SELECT * FROM bill_amounts WHERE bill_id IN (SELECT id FROM bills WHERE group_id = @g)`,
	"financing_events":   `SELECT * FROM financing_events WHERE financing_id IN (SELECT id FROM financings WHERE group_id = @g)`,
	"savings_moves":      `SELECT * FROM savings_moves WHERE box_id IN (SELECT id FROM savings_boxes WHERE group_id = @g)`,
	"alelo_credits":      `SELECT * FROM alelo_credits WHERE alelo_id IN (SELECT id FROM alelo_cards WHERE ` + ofMembers + `)`,
	"prices":             `SELECT * FROM prices WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"receipts":           `SELECT * FROM receipts WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"receipt_links":      `SELECT * FROM receipt_links WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"list_items":         `SELECT * FROM list_items WHERE list_id IN (SELECT id FROM shopping_lists WHERE group_id = @g)`,

	// Shared by everyone: the category tree has no group.
	"categories": `SELECT * FROM categories`,

	// Personal data of the user who asks: rules, import history and read warnings.
	"alias_rules":        `SELECT * FROM alias_rules WHERE created_by = @u`,
	"imports":            `SELECT * FROM imports WHERE created_by = @u AND card_id IN (SELECT id FROM cards WHERE ` + ofMembers + `)`,
	"notification_reads": `SELECT * FROM notification_reads WHERE user_id = @u`,
}

// WriteZip writes one CSV per table with the rows of the active group, from a single consistent read snapshot.
func WriteZip(ctx context.Context, conn *sql.DB, w io.Writer) error {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return err
	}
	args := []any{sql.Named("g", groupID), sql.Named("u", auth.UserID(ctx))}
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
		query, err := tableQuery(ctx, tx, table)
		if err != nil {
			return err
		}
		f, err := zw.Create(table + ".csv")
		if err != nil {
			return err
		}
		if err := writeTable(ctx, tx, query, args, f); err != nil {
			return fmt.Errorf("export %s: %w", table, err)
		}
	}
	return zw.Close()
}

// tableQuery returns the query that selects the rows of a table for the active group, or an error when the table has no rule.
func tableQuery(ctx context.Context, tx *sql.Tx, table string) (string, error) {
	if table == "users" {
		return usersQuery(ctx, tx)
	}
	q, ok := rules[table]
	if !ok {
		return "", fmt.Errorf("export: table %s has no rule for groups", table)
	}
	return q, nil
}

// usersQuery selects the users who are members of the group, leaving out the PIN columns, which never leave the database.
func usersQuery(ctx context.Context, tx *sql.Tx) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info('users') ORDER BY cid`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		if !strings.HasPrefix(name, "pin_") {
			cols = append(cols, `"`+name+`"`)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return `SELECT ` + strings.Join(cols, ", ") + ` FROM users WHERE ` + ofMembers, nil
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
	sort.Strings(out)
	return out, rows.Err()
}

// writeTable writes the header and every row of a query as CSV.
func writeTable(ctx context.Context, tx *sql.Tx, query string, args []any, w io.Writer) error {
	// The query comes from the rules above, not from the user.
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	record := make([]string, len(cols))
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for i, v := range values {
			record[i] = formatValue(v)
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
