-- Import of the account statement of Santander (RF-86 to RF-90, RN-32).

-- name: CreateStatementImport :one
INSERT INTO statement_imports (group_id, account_id, file_name, line_count, closing_cents, created_by, created_at)
VALUES (CAST(sqlc.arg(group_id) AS INTEGER), sqlc.arg(account_id), sqlc.arg(file_name), sqlc.arg(line_count), sqlc.arg(closing_cents), sqlc.arg(created_by), sqlc.arg(created_at))
RETURNING *;

-- name: ListStatementImports :many
SELECT i.*, a.name AS account_name, p.name AS created_by_name, g.name AS group_name,
       CAST((SELECT count(*) FROM entries e WHERE e.statement_import_id = i.id) AS INTEGER) AS entry_count
FROM statement_imports i
JOIN accounts a ON a.id = i.account_id
JOIN users u ON u.id = i.created_by
JOIN persons p ON p.id = u.person_id
JOIN groups g ON g.id = i.group_id
-- The imports of the user in every space, since each one records in the space chosen on the screen (RF-83).
WHERE i.created_by = sqlc.arg(created_by)
ORDER BY i.id DESC;

-- name: GetStatementImport :one
SELECT *
FROM statement_imports
WHERE id = sqlc.arg(id) AND created_by = sqlc.arg(created_by);

-- name: DeleteStatementImportEntries :exec
-- Invoice payments and member divisions of these entries go with them by cascade and trigger.
DELETE FROM entries
WHERE statement_import_id = sqlc.arg(import_id);

-- name: DeleteStatementImport :exec
DELETE FROM statement_imports
WHERE id = sqlc.arg(id) AND created_by = sqlc.arg(created_by);

-- name: SetEntryStatementImport :exec
UPDATE entries
SET statement_import_id = sqlc.arg(import_id)
WHERE id = sqlc.arg(id);

-- name: AccountEntriesNear :many
-- Confirmed entries of the account with the given signed amount within a day of the date, in every space, for the
-- duplicates of a statement line (RN-32).
SELECT e.id, e.description, e.occurred_on, e.amount_cents, CAST(coalesce(e.group_id, 0) AS INTEGER) AS group_id
FROM entries e
WHERE e.account_id = sqlc.arg(account_id) AND e.confirmed
  AND e.amount_cents = sqlc.arg(amount_cents)
  AND e.occurred_on >= sqlc.arg(first_day) AND e.occurred_on <= sqlc.arg(last_day)
ORDER BY e.occurred_on, e.id;

-- name: PersonalGroupOf :one
-- The personal space of a person, where the invoices of her cards are paid (RF-83).
SELECT id
FROM groups
WHERE kind = 'personal' AND owner_person_id = sqlc.arg(person_id);
