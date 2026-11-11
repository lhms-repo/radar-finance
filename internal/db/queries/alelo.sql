-- name: ListAleloCards :many
SELECT c.*, p.name AS person_name,
       CAST(c.initial_balance_cents
            + coalesce((SELECT sum(amount_cents) FROM alelo_credits cr WHERE cr.alelo_id = c.id), 0)
            - coalesce((SELECT sum(amount_cents) FROM alelo_purchases pu WHERE pu.alelo_id = c.id), 0) AS INTEGER) AS balance_cents
FROM alelo_cards c
JOIN persons p ON p.id = c.person_id
ORDER BY c.active DESC, c.id;

-- name: GetAleloCard :one
SELECT *
FROM alelo_cards
WHERE id = ?;

-- name: CreateAleloCard :one
INSERT INTO alelo_cards (name, person_id, monthly_credit_cents, credit_day, initial_balance_cents, start_month,
                         style, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAleloCard :exec
UPDATE alelo_cards
SET name = ?, person_id = ?, monthly_credit_cents = ?, credit_day = ?, style = ?, active = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteAleloCard :exec
DELETE FROM alelo_cards
WHERE id = ?;

-- name: AleloBalanceBefore :one
SELECT CAST(c.initial_balance_cents
            + coalesce((SELECT sum(amount_cents) FROM alelo_credits cr WHERE cr.alelo_id = c.id AND cr.month < sqlc.arg(month)), 0)
            - coalesce((SELECT sum(amount_cents) FROM alelo_purchases pu WHERE pu.alelo_id = c.id AND pu.purchased_on < sqlc.arg(first_day)), 0) AS INTEGER) AS balance_cents
FROM alelo_cards c
WHERE c.id = sqlc.arg(alelo_id);

-- name: GetAleloCredit :one
SELECT *
FROM alelo_credits
WHERE alelo_id = ? AND month = ?;

-- name: CreateAleloCredit :exec
INSERT INTO alelo_credits (alelo_id, month, amount_cents, credited_on, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteAleloCredit :execrows
DELETE FROM alelo_credits
WHERE alelo_id = ? AND month = ?;

-- name: ListAleloPurchases :many
SELECT pu.*, p.name AS created_by_name
FROM alelo_purchases pu
JOIN users u ON u.id = pu.created_by
JOIN persons p ON p.id = u.person_id
WHERE pu.alelo_id = sqlc.arg(alelo_id) AND pu.purchased_on >= sqlc.arg(first_day) AND pu.purchased_on <= sqlc.arg(last_day)
ORDER BY pu.purchased_on DESC, pu.id DESC;

-- name: CreateAleloPurchase :one
INSERT INTO alelo_purchases (alelo_id, purchased_on, description, amount_cents, category_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAleloPurchase :one
SELECT *
FROM alelo_purchases
WHERE id = ?;

-- name: DeleteAleloPurchase :exec
DELETE FROM alelo_purchases
WHERE id = ?;

-- name: GetCategoryBySystemKey :one
SELECT id
FROM categories
WHERE system_key = ?;
