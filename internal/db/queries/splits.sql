-- name: ListParties :many
SELECT id, name
FROM parties
ORDER BY name COLLATE NOCASE;

-- name: GetPartyByName :one
SELECT id, name
FROM parties
WHERE name = ? COLLATE NOCASE;

-- name: CreateParty :one
INSERT INTO parties (name, created_by, created_at)
VALUES (?, ?, ?)
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
WHERE si.card_id = ? AND si.invoice_month = ?
ORDER BY p.name COLLATE NOCASE;
