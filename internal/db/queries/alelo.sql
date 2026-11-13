-- name: ListAleloCards :many
-- The balance is real, with the purchases of every space; the API shows it only to the holder (RF-83).
SELECT c.*, p.name AS person_name,
       CAST(c.initial_balance_cents
            + coalesce((SELECT sum(amount_cents) FROM alelo_credits cr WHERE cr.alelo_id = c.id), 0)
            - coalesce((SELECT sum(amount_cents) FROM alelo_purchases pu WHERE pu.alelo_id = c.id), 0) AS INTEGER) AS balance_cents
FROM alelo_cards c
JOIN persons p ON p.id = c.person_id
WHERE EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id)
ORDER BY c.active DESC, c.id;

-- name: GetAleloCard :one
SELECT c.*
FROM alelo_cards c
WHERE c.id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id);

-- name: CreateAleloCard :one
INSERT INTO alelo_cards (name, person_id, monthly_credit_cents, credit_day, initial_balance_cents, start_month,
                         style, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAleloCard :exec
UPDATE alelo_cards
SET name = ?, person_id = ?, monthly_credit_cents = ?, credit_day = ?, style = ?, active = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = alelo_cards.person_id);

-- name: DeleteAleloCard :exec
DELETE FROM alelo_cards
WHERE id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = alelo_cards.person_id);

-- name: AleloBalanceBefore :one
-- Real balance before a month, with the purchases of every space, read by the holder in her personal space (RF-83).
SELECT CAST(c.initial_balance_cents
            + coalesce((SELECT sum(amount_cents) FROM alelo_credits cr WHERE cr.alelo_id = c.id AND cr.month < sqlc.arg(month)), 0)
            - coalesce((SELECT sum(amount_cents) FROM alelo_purchases pu WHERE pu.alelo_id = c.id AND pu.purchased_on < sqlc.arg(first_day)), 0) AS INTEGER) AS balance_cents
FROM alelo_cards c
WHERE c.id = sqlc.arg(alelo_id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id);

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
SELECT pu.*, p.name AS created_by_name,
       coalesce(g.name, '') AS group_name
FROM alelo_purchases pu
JOIN users u ON u.id = pu.created_by
JOIN persons p ON p.id = u.person_id
LEFT JOIN groups g ON g.id = pu.group_id
-- With full set, the holder reads the purchases of every space in her personal space (RF-83).
WHERE (CAST(sqlc.arg(full) AS INTEGER) = 1 OR pu.group_id = CAST(sqlc.arg(group_id) AS INTEGER)) AND pu.alelo_id = sqlc.arg(alelo_id) AND pu.purchased_on >= sqlc.arg(first_day) AND pu.purchased_on <= sqlc.arg(last_day)
ORDER BY pu.purchased_on DESC, pu.id DESC;

-- name: CreateAleloPurchase :one
INSERT INTO alelo_purchases (alelo_id, purchased_on, description, amount_cents, category_id, created_by, created_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: GetAleloPurchase :one
SELECT *
FROM alelo_purchases
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteAleloPurchase :exec
DELETE FROM alelo_purchases
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: GetCategoryBySystemKey :one
SELECT id
FROM categories
WHERE system_key = ?;

-- name: AleloUsedElsewhere :one
-- Whether the Alelo card has purchases in a group other than this one (RF-82).
SELECT CAST(EXISTS (SELECT 1 FROM alelo_purchases ap WHERE ap.alelo_id = sqlc.arg(alelo_id) AND coalesce(ap.group_id, 0) <> CAST(sqlc.arg(group_id) AS INTEGER)) AS INTEGER) AS used;
