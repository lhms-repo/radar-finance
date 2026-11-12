-- name: ListSavingsBoxes :many
SELECT *
FROM savings_boxes
ORDER BY id;

-- name: GetSavingsBox :one
SELECT *
FROM savings_boxes
WHERE id = ?;

-- name: CreateSavingsBox :one
INSERT INTO savings_boxes (name, target_cents, percent_bp, initial_balance_cents, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateSavingsBox :exec
UPDATE savings_boxes
SET name = ?, target_cents = ?, percent_bp = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteSavingsBox :exec
DELETE FROM savings_boxes
WHERE id = ?;

-- name: ListSavingsMoves :many
SELECT m.*, e.occurred_on, a.name AS account_name, p.name AS created_by_name
FROM savings_moves m
JOIN entries e ON e.id = m.entry_id
JOIN accounts a ON a.id = e.account_id
JOIN users u ON u.id = m.created_by
JOIN persons p ON p.id = u.person_id
ORDER BY m.box_id, e.occurred_on, m.id;

-- name: CreateSavingsMove :exec
INSERT INTO savings_moves (box_id, kind, month, amount_cents, reason, entry_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: SavingsBalanceWithoutEntry :one
-- Balance of the box of a savings entry as it would be without that entry.
SELECT CAST(b.initial_balance_cents + coalesce(sum(CASE m.kind WHEN 'deposit' THEN m.amount_cents ELSE -m.amount_cents END), 0) AS INTEGER) AS balance_cents
FROM savings_moves d
JOIN savings_boxes b ON b.id = d.box_id
LEFT JOIN savings_moves m ON m.box_id = d.box_id AND m.entry_id <> d.entry_id
WHERE d.entry_id = ?
GROUP BY b.id;
