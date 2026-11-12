-- name: ListFinancings :many
SELECT f.*, a.name AS account_name, c.name AS category_name, coalesce(pc.icon, '') AS category_icon
FROM financings f
JOIN accounts a ON a.id = f.account_id
JOIN categories c ON c.id = f.category_id
LEFT JOIN categories pc ON pc.id = c.parent_id
WHERE f.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY f.due_day, f.name COLLATE NOCASE, f.id;

-- name: GetFinancing :one
SELECT *
FROM financings
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateFinancing :one
INSERT INTO financings (name, category_id, account_id, due_day, installment_cents, total_installments, initial_paid,
                        initial_balance_cents, start_month, created_by, created_at, updated_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: UpdateFinancing :exec
UPDATE financings
SET name = ?, category_id = ?, account_id = ?, due_day = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: SetFinancingInstallment :exec
UPDATE financings
SET installment_cents = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteFinancing :exec
DELETE FROM financings
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: ListFinancingEvents :many
SELECT ev.*, coalesce(e.amount_cents, 0) AS entry_amount_cents, coalesce(e.occurred_on, '') AS paid_on,
       coalesce(a.name, '') AS paid_from
FROM financing_events ev
JOIN financings f ON f.id = ev.financing_id
LEFT JOIN entries e ON e.id = ev.entry_id
LEFT JOIN accounts a ON a.id = e.account_id
WHERE f.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY ev.financing_id, ev.id;

-- name: CreateFinancingEvent :exec
INSERT INTO financing_events (financing_id, kind, month, number, entry_id, balance_after_cents, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
