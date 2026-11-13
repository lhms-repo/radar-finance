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

// ofUser keeps the rows held by the person of the user asking, like the credits of her own Alelo cards (RF-83).
const ofUser = `person_id = (SELECT person_id FROM users WHERE id = @u)`

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
	"group_weights":   `SELECT * FROM group_weights WHERE group_id = @g`,
	"settlements":     `SELECT * FROM settlements WHERE group_id = @g`,
	"member_split_shares": `SELECT * FROM member_split_shares WHERE split_id IN (SELECT id FROM member_splits ms WHERE` +
		` (ms.item_kind = 'entry' AND ms.item_id IN (SELECT id FROM entries WHERE group_id = @g))` +
		` OR (ms.item_kind = 'purchase' AND ms.item_id IN (SELECT id FROM purchases WHERE group_id = @g))` +
		` OR (ms.item_kind = 'recurrence' AND ms.item_id IN (SELECT id FROM recurrences WHERE group_id = @g))` +
		` OR (ms.item_kind = 'bill' AND ms.item_id IN (SELECT id FROM bills WHERE group_id = @g))` +
		` OR (ms.item_kind = 'financing' AND ms.item_id IN (SELECT id FROM financings WHERE group_id = @g))` +
		` OR (ms.item_kind = 'alelo' AND ms.item_id IN (SELECT id FROM alelo_purchases WHERE group_id = @g)))`,
	"member_splits": `SELECT * FROM member_splits ms WHERE` +
		` (ms.item_kind = 'entry' AND ms.item_id IN (SELECT id FROM entries WHERE group_id = @g))` +
		` OR (ms.item_kind = 'purchase' AND ms.item_id IN (SELECT id FROM purchases WHERE group_id = @g))` +
		` OR (ms.item_kind = 'recurrence' AND ms.item_id IN (SELECT id FROM recurrences WHERE group_id = @g))` +
		` OR (ms.item_kind = 'bill' AND ms.item_id IN (SELECT id FROM bills WHERE group_id = @g))` +
		` OR (ms.item_kind = 'financing' AND ms.item_id IN (SELECT id FROM financings WHERE group_id = @g))` +
		` OR (ms.item_kind = 'alelo' AND ms.item_id IN (SELECT id FROM alelo_purchases WHERE group_id = @g))`,

	// Rows of the people of the group.
	"persons":  `SELECT * FROM persons WHERE id IN (` + members + `)`,
	"accounts": `SELECT * FROM accounts WHERE ` + ofMembers,
	"cards":    `SELECT * FROM cards WHERE ` + ofMembers,
	// The starting balance goes only to the holder, see aleloCardsQuery.
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
	// Credits of an Alelo card go only to its holder, like its balance (RF-83).
	"alelo_credits": `SELECT * FROM alelo_credits WHERE alelo_id IN (SELECT id FROM alelo_cards WHERE ` + ofUser + `)`,
	"prices":        `SELECT * FROM prices WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"receipts":      `SELECT * FROM receipts WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"receipt_links": `SELECT * FROM receipt_links WHERE market_id IN (SELECT id FROM markets WHERE group_id = @g)`,
	"list_items":    `SELECT * FROM list_items WHERE list_id IN (SELECT id FROM shopping_lists WHERE group_id = @g)`,

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
	switch table {
	case "users":
		return usersQuery(ctx, tx)
	case "alelo_cards":
		return aleloCardsQuery(ctx, tx)
	}
	q, ok := rules[table]
	if !ok {
		return "", fmt.Errorf("export: table %s has no rule for groups", table)
	}
	return q, nil
}

// usersQuery selects the users who are members of the group, leaving out the PIN columns, which never leave the database.
func usersQuery(ctx context.Context, tx *sql.Tx) (string, error) {
	cols, err := columns(ctx, tx, "users")
	if err != nil {
		return "", err
	}
	var kept []string
	for _, c := range cols {
		if !strings.HasPrefix(c, "pin_") {
			kept = append(kept, `"`+c+`"`)
		}
	}
	return `SELECT ` + strings.Join(kept, ", ") + ` FROM users WHERE ` + ofMembers, nil
}

// aleloCardsQuery selects the Alelo cards of the members with the starting balance only on the cards of the user
// asking, since the balance of a card is shown only to its holder (RF-83).
func aleloCardsQuery(ctx context.Context, tx *sql.Tx) (string, error) {
	cols, err := columns(ctx, tx, "alelo_cards")
	if err != nil {
		return "", err
	}
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		if c == "initial_balance_cents" {
			out = append(out, `CASE WHEN `+ofUser+` THEN initial_balance_cents END AS initial_balance_cents`)
			continue
		}
		out = append(out, `"`+c+`"`)
	}
	return `SELECT ` + strings.Join(out, ", ") + ` FROM alelo_cards WHERE ` + ofMembers, nil
}

// columns lists the columns of a table in their order.
func columns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
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
