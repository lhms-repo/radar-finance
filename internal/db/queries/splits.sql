-- name: ListParties :many
SELECT id, name
FROM parties
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY name COLLATE NOCASE;

-- name: GetPartyByName :one
SELECT id, name
FROM parties
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND name = sqlc.arg(name) COLLATE NOCASE;

-- name: CreateParty :one
INSERT INTO parties (name, created_by, created_at, group_id)
VALUES (?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING id, name;

-- name: CreateSplit :one
INSERT INTO splits (purchase_id, party_id, share_cents)
VALUES (?, ?, ?)
RETURNING id;

-- name: DeleteSplits :exec
DELETE FROM splits
WHERE purchase_id = ?;

-- name: CreateSplitInstallment :exec
INSERT INTO split_installments (split_id, number, card_id, invoice_month, amount_cents)
VALUES (?, ?, ?, ?, ?);

-- name: ListPurchaseSplits :many
SELECT s.id, s.party_id, p.name AS party_name, s.share_cents
FROM splits s
JOIN parties p ON p.id = s.party_id
WHERE s.purchase_id = ?
ORDER BY p.name COLLATE NOCASE;

-- name: CountPaidSplitInstallments :one
SELECT count(*)
FROM split_installments si
JOIN splits s ON s.id = si.split_id
WHERE s.purchase_id = ? AND si.entry_id IS NOT NULL;

-- name: ListInvoiceSplitInstallments :many
SELECT si.id, si.number, si.amount_cents, si.entry_id, s.purchase_id, p.name AS party_name
FROM split_installments si
JOIN splits s ON s.id = si.split_id
JOIN parties p ON p.id = s.party_id
JOIN purchases pu ON pu.id = s.purchase_id
WHERE pu.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND si.card_id = sqlc.arg(card_id) AND si.invoice_month = sqlc.arg(invoice_month)
ORDER BY p.name COLLATE NOCASE;

-- name: ListReceivables :many
SELECT si.id, si.number, si.invoice_month, si.amount_cents, si.entry_id, s.party_id, pa.name AS party_name,
       pu.id AS purchase_id, pu.merchant, pu.purchased_on, pu.installment_count, c.name AS card_name, c.closing_day, c.due_day,
       coalesce(e.occurred_on, '') AS received_on, coalesce(a.name, '') AS received_in
FROM split_installments si
JOIN splits s ON s.id = si.split_id
JOIN parties pa ON pa.id = s.party_id
JOIN purchases pu ON pu.id = s.purchase_id
JOIN cards c ON c.id = si.card_id
LEFT JOIN entries e ON e.id = si.entry_id
LEFT JOIN accounts a ON a.id = e.account_id
WHERE pu.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY pa.name COLLATE NOCASE, si.invoice_month, pu.merchant COLLATE NOCASE, si.number;

-- name: SetSplitInstallmentEntry :exec
UPDATE split_installments
SET entry_id = ?
WHERE id = ?;
