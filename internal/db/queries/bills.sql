-- name: ListBills :many
SELECT b.*, a.name AS account_name, c.name AS category_name, coalesce(pc.name, '') AS parent_category_name,
       coalesce(pc.icon, '') AS category_icon
FROM bills b
JOIN accounts a ON a.id = b.account_id
JOIN categories c ON c.id = b.category_id
LEFT JOIN categories pc ON pc.id = c.parent_id
ORDER BY b.due_day, b.name COLLATE NOCASE, b.id;

-- name: GetBill :one
SELECT *
FROM bills
WHERE id = ?;

-- name: CreateBill :one
INSERT INTO bills (name, category_id, account_id, due_day, expected_cents, start_month, end_month,
                   created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateBill :exec
UPDATE bills
SET name = ?, category_id = ?, account_id = ?, due_day = ?, expected_cents = ?, start_month = ?, end_month = ?,
    updated_at = ?
WHERE id = ?;

-- name: DeleteBill :exec
DELETE FROM bills
WHERE id = ?;

-- name: ListBillAmounts :many
SELECT bill_id, actual_cents
FROM bill_amounts
WHERE month = ?;

-- name: UpsertBillAmount :exec
INSERT INTO bill_amounts (bill_id, month, actual_cents)
VALUES (?, ?, ?)
ON CONFLICT (bill_id, month) DO UPDATE SET actual_cents = excluded.actual_cents;

-- name: DeleteBillAmount :exec
DELETE FROM bill_amounts
WHERE bill_id = ? AND month = ?;

-- name: ListBillPayments :many
SELECT e.id, e.bill_id, e.amount_cents, e.occurred_on, e.account_id, a.name AS account_name
FROM entries e
JOIN accounts a ON a.id = e.account_id
WHERE e.bill_id IS NOT NULL AND e.bill_month = ?;

-- name: CreateBillEntry :one
INSERT INTO entries (account_id, kind, amount_cents, occurred_on, confirmed, category_id, description, bill_id, bill_month,
                     created_by, created_at, updated_at)
VALUES (?, 'expense', ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;
