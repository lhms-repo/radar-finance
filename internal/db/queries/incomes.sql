-- name: ListIncomes :many
SELECT i.*, p.name AS person_name, a.name AS account_name
FROM incomes i
JOIN persons p ON p.id = i.person_id
JOIN accounts a ON a.id = i.account_id
ORDER BY i.kind DESC, i.person_id, i.name COLLATE NOCASE, i.id;

-- name: GetIncome :one
SELECT *
FROM incomes
WHERE id = ?;

-- name: CreateIncome :one
INSERT INTO incomes (kind, name, person_id, account_id, amount_cents, day, start_month, end_on, expected_on,
                     created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateIncome :exec
UPDATE incomes
SET name = ?, person_id = ?, account_id = ?, amount_cents = ?, day = ?, start_month = ?, end_on = ?,
    expected_on = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteIncome :exec
DELETE FROM incomes
WHERE id = ?;

-- name: ListIncomeConfirmations :many
SELECT e.id, e.income_id, e.income_month, e.amount_cents, e.occurred_on, e.account_id, a.name AS account_name
FROM entries e
JOIN accounts a ON a.id = e.account_id
WHERE e.income_id IS NOT NULL AND e.income_month = ?;

-- name: ListOneOffIncomes :many
SELECT e.id, e.amount_cents, e.occurred_on, e.description, e.account_id, a.name AS account_name
FROM entries e
JOIN accounts a ON a.id = e.account_id
WHERE e.kind = 'income' AND e.income_id IS NULL AND e.confirmed
  AND e.occurred_on >= sqlc.arg(first_day) AND e.occurred_on <= sqlc.arg(last_day)
ORDER BY e.occurred_on, e.id;

-- name: CreateIncomeEntry :one
INSERT INTO entries (account_id, kind, amount_cents, occurred_on, confirmed, description, income_id, income_month,
                     created_by, created_at, updated_at)
VALUES (?, 'income', ?, ?, 1, ?, ?, ?, ?, ?, ?)
RETURNING id;
