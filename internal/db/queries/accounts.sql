-- name: GetGroupPerson :one
-- A person who is a member of the group (RF-78); anyone else counts as missing.
SELECT p.id, p.name
FROM persons p
JOIN group_members gm ON gm.person_id = p.id
WHERE p.id = sqlc.arg(id) AND gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: ListAccounts :many
-- Accounts held by members of the group, with the balance made of the group's entries only (RF-82).
SELECT a.id, a.name, a.person_id, p.name AS person_name, a.initial_balance_cents, a.active,
       CAST(a.initial_balance_cents + coalesce((
           SELECT sum(e.amount_cents) FROM entries e
           WHERE e.account_id = a.id AND e.confirmed AND e.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
       ), 0) AS INTEGER) AS balance_cents
FROM accounts a
JOIN persons p ON p.id = a.person_id
WHERE EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = a.person_id)
ORDER BY a.active DESC, a.id;

-- name: GetAccount :one
SELECT a.*
FROM accounts a
WHERE a.id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = a.person_id);

-- name: CreateAccount :one
INSERT INTO accounts (name, person_id, initial_balance_cents, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAccount :exec
UPDATE accounts
SET name = ?, person_id = ?, initial_balance_cents = ?, active = ?, updated_at = ?
WHERE id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = accounts.person_id);

-- name: DeleteAccount :exec
DELETE FROM accounts
WHERE id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = accounts.person_id);

-- name: AccountBalanceBefore :one
SELECT CAST(a.initial_balance_cents + coalesce((
           SELECT sum(e.amount_cents) FROM entries e
           WHERE e.account_id = a.id AND e.confirmed AND e.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND e.occurred_on < sqlc.arg(day)
       ), 0) AS INTEGER) AS balance_cents
FROM accounts a
WHERE a.id = sqlc.arg(account_id)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = a.person_id);

-- name: AccountBalance :one
SELECT CAST(a.initial_balance_cents + coalesce((
           SELECT sum(e.amount_cents) FROM entries e
           WHERE e.account_id = a.id AND e.confirmed AND e.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
       ), 0) AS INTEGER) AS balance_cents
FROM accounts a
WHERE a.id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = a.person_id);

-- name: CreateEntry :one
INSERT INTO entries (account_id, kind, amount_cents, occurred_on, confirmed, category_id, description,
                     transfer_id, created_by, created_at, updated_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: GetEntry :one
SELECT *
FROM entries
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteEntry :exec
DELETE FROM entries
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateTransfer :one
INSERT INTO transfers (from_account_id, to_account_id, amount_cents, occurred_on, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteTransfer :exec
-- Only a transfer whose entries belong to the group goes away.
DELETE FROM transfers
WHERE transfers.id = sqlc.arg(id)
  AND EXISTS (SELECT 1 FROM entries e WHERE e.transfer_id = transfers.id AND e.group_id = CAST(sqlc.arg(group_id) AS INTEGER));

-- name: ListAccountEntries :many
SELECT e.id, e.account_id, e.kind, e.amount_cents, e.occurred_on, e.confirmed, e.category_id,
       e.description, e.transfer_id, e.created_at,
       coalesce(c.name, '') AS category_name,
       coalesce(pc.name, '') AS parent_category_name,
       coalesce(pc.icon, c.icon, '') AS category_icon,
       CAST(coalesce((
           SELECT oa.name FROM entries o JOIN accounts oa ON oa.id = o.account_id
           WHERE o.transfer_id = e.transfer_id AND o.id <> e.id AND o.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
       ), '') AS TEXT) AS counterpart_account_name,
       p.name AS created_by_name
FROM entries e
LEFT JOIN categories c ON c.id = e.category_id
LEFT JOIN categories pc ON pc.id = c.parent_id
JOIN users u ON u.id = e.created_by
JOIN persons p ON p.id = u.person_id
WHERE e.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND e.account_id = sqlc.arg(account_id)
  AND e.occurred_on >= sqlc.arg(first_day) AND e.occurred_on <= sqlc.arg(last_day)
ORDER BY e.occurred_on DESC, e.id DESC;

-- name: MonthFlow :one
SELECT CAST(coalesce(sum(CASE WHEN kind = 'income' THEN amount_cents END), 0) AS INTEGER) AS income_cents,
       CAST(coalesce(-sum(CASE WHEN kind = 'expense' THEN amount_cents END), 0) AS INTEGER) AS expense_cents
FROM entries
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND confirmed AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day);
