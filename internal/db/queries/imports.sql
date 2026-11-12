-- name: ListAliasRules :many
SELECT r.*, coalesce(c.name, '') AS category_name
FROM alias_rules r
LEFT JOIN categories c ON c.id = r.category_id
ORDER BY r.original, r.id DESC;

-- name: GetAliasRule :one
SELECT *
FROM alias_rules
WHERE id = ?;

-- name: CreateAliasRule :one
INSERT INTO alias_rules (original, friendly_name, category_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAliasRule :exec
UPDATE alias_rules
SET original = ?, friendly_name = ?, category_id = ?
WHERE id = ?;

-- name: DeleteAliasRule :exec
DELETE FROM alias_rules
WHERE id = ?;

-- name: CreateImport :one
INSERT INTO imports (card_id, invoice_month, file_name, line_count, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListImports :many
SELECT i.*, c.name AS card_name, p.name AS created_by_name,
       CAST((SELECT count(*) FROM purchases pu WHERE pu.import_id = i.id) AS INTEGER) AS purchase_count
FROM imports i
JOIN cards c ON c.id = i.card_id
JOIN users u ON u.id = i.created_by
JOIN persons p ON p.id = u.person_id
ORDER BY i.id DESC;

-- name: GetImport :one
SELECT *
FROM imports
WHERE id = ?;

-- name: ListImportPurchases :many
SELECT id
FROM purchases
WHERE import_id = ?;

-- name: DeleteImport :exec
DELETE FROM imports
WHERE id = ?;

-- name: SetPurchaseImport :exec
UPDATE purchases
SET import_id = ?
WHERE id = ?;

-- name: CardItemsNear :many
-- One-shot purchases of the card bought within a day of the given date, for duplicates of one-shot CSV lines (RN-20).
-- Installment purchases are left out: their CSV lines carry Parcela N/M and match by number (RF-57).
SELECT i.id AS installment_id, p.id AS purchase_id, p.merchant, p.purchased_on, p.kind, i.amount_cents, i.number, p.installment_count, i.invoice_month
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE i.card_id = sqlc.arg(card_id)
  AND p.installment_count = 1
  AND p.purchased_on >= sqlc.arg(first_day) AND p.purchased_on <= sqlc.arg(last_day)
ORDER BY i.id;

-- name: CardInstallmentMatch :many
-- Installments of the card on the invoice with the given number, count and amount, to link a CSV installment (RF-57).
SELECT i.id AS installment_id, p.merchant, p.purchased_on, i.amount_cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE i.card_id = sqlc.arg(card_id)
  AND i.invoice_month = sqlc.arg(invoice_month)
  AND i.number = sqlc.arg(number)
  AND p.installment_count = sqlc.arg(installment_count)
  AND i.amount_cents = sqlc.arg(amount_cents)
ORDER BY i.id;
