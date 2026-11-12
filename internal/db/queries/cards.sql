-- name: ListCards :many
SELECT c.*, p.name AS person_name
FROM cards c
JOIN persons p ON p.id = c.person_id
WHERE EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id)
ORDER BY c.active DESC, c.id;

-- name: GetCard :one
SELECT c.*
FROM cards c
WHERE c.id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id);

-- name: CreateCard :one
INSERT INTO cards (name, person_id, limit_cents, closing_day, due_day, last_digits, color, finish,
                   created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCard :exec
UPDATE cards
SET name = ?, person_id = ?, limit_cents = ?, closing_day = ?, due_day = ?, last_digits = ?, color = ?,
    finish = ?, active = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = cards.person_id);

-- name: DeleteCard :exec
DELETE FROM cards
WHERE id = sqlc.arg(id) AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = cards.person_id);

-- name: CreatePurchase :one
INSERT INTO purchases (card_id, kind, purchased_on, merchant, description, category_id, buyer_person_id,
                       total_cents, installment_count, first_invoice, created_by, created_at, updated_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: UpdatePurchase :exec
UPDATE purchases
SET purchased_on = ?, merchant = ?, description = ?, category_id = ?, buyer_person_id = ?, total_cents = ?,
    installment_count = ?, first_invoice = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: GetPurchase :one
SELECT *
FROM purchases
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeletePurchase :exec
DELETE FROM purchases
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateInstallment :exec
INSERT INTO installments (purchase_id, card_id, invoice_month, number, amount_cents)
VALUES (?, ?, ?, ?, ?);

-- name: DeleteInstallments :exec
DELETE FROM installments
WHERE purchase_id = ?;

-- name: ListInvoiceInstallments :many
SELECT i.id, i.purchase_id, i.number, i.amount_cents, p.kind, p.purchased_on, p.merchant, p.description,
       p.category_id, p.installment_count, p.total_cents, p.buyer_person_id,
       coalesce(c.name, '') AS category_name,
       coalesce(pc.name, '') AS parent_category_name,
       coalesce(pc.icon, '') AS category_icon,
       coalesce(bp.name, '') AS buyer_name,
       up.name AS created_by_name
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN categories pc ON pc.id = c.parent_id
LEFT JOIN persons bp ON bp.id = p.buyer_person_id
JOIN users u ON u.id = p.created_by
JOIN persons up ON up.id = u.person_id
WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND i.card_id = sqlc.arg(card_id) AND i.invoice_month = sqlc.arg(invoice_month)
ORDER BY p.purchased_on DESC, i.id DESC;

-- name: SumInstallmentsByMonth :many
SELECT i.invoice_month, CAST(sum(i.amount_cents) AS INTEGER) AS total_cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND i.card_id = sqlc.arg(card_id)
GROUP BY i.invoice_month
ORDER BY i.invoice_month;

-- name: ListCardRecurrences :many
SELECT r.*, coalesce(c.name, '') AS category_name, coalesce(pc.name, '') AS parent_category_name,
       coalesce(pc.icon, '') AS category_icon
FROM recurrences r
JOIN categories c ON c.id = r.category_id
LEFT JOIN categories pc ON pc.id = c.parent_id
WHERE r.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND r.card_id = sqlc.arg(card_id)
ORDER BY r.description COLLATE NOCASE, r.id;

-- name: GetRecurrence :one
SELECT *
FROM recurrences
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: CreateRecurrence :one
INSERT INTO recurrences (card_id, description, category_id, start_month, end_month, created_by, created_at, updated_at, group_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
RETURNING *;

-- name: UpdateRecurrence :exec
UPDATE recurrences
SET description = ?, category_id = ?, end_month = ?, updated_at = ?
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: DeleteRecurrence :exec
DELETE FROM recurrences
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: ListRecurrencePrices :many
SELECT rp.*
FROM recurrence_prices rp
JOIN recurrences r ON r.id = rp.recurrence_id
WHERE r.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND r.card_id = sqlc.arg(card_id)
ORDER BY rp.recurrence_id, rp.from_month;

-- name: UpsertRecurrencePrice :exec
INSERT INTO recurrence_prices (recurrence_id, from_month, amount_cents)
VALUES (?, ?, ?)
ON CONFLICT (recurrence_id, from_month) DO UPDATE SET amount_cents = excluded.amount_cents;

-- name: DeleteRecurrencePricesAfter :exec
DELETE FROM recurrence_prices
WHERE recurrence_id = ? AND from_month > ?;

-- name: ListCardPayments :many
SELECT ip.id, ip.invoice_month, ip.entry_id, ip.amount_cents, e.occurred_on, e.account_id, a.name AS account_name
FROM invoice_payments ip
JOIN entries e ON e.id = ip.entry_id
JOIN accounts a ON a.id = e.account_id
WHERE ip.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND e.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND ip.card_id = sqlc.arg(card_id)
ORDER BY ip.invoice_month;

-- name: CreateInvoicePayment :exec
INSERT INTO invoice_payments (group_id, card_id, invoice_month, entry_id, amount_cents, created_by, created_at)
VALUES (CAST(sqlc.arg(group_id) AS INTEGER), sqlc.arg(card_id), sqlc.arg(invoice_month), sqlc.arg(entry_id), sqlc.arg(amount_cents), sqlc.arg(created_by), sqlc.arg(created_at));

-- name: FirstInstallmentNumber :one
SELECT CAST(coalesce(min(number), 1) AS INTEGER) AS number
FROM installments
WHERE purchase_id = ?;

-- name: CardUsedElsewhere :one
-- Whether the card has purchases, recurrences or payments in a group other than this one (RF-82).
SELECT CAST(EXISTS (SELECT 1 FROM purchases p WHERE p.card_id = sqlc.arg(card_id) AND coalesce(p.group_id, 0) <> CAST(sqlc.arg(group_id) AS INTEGER))
    OR EXISTS (SELECT 1 FROM recurrences r WHERE r.card_id = sqlc.arg(card_id) AND coalesce(r.group_id, 0) <> CAST(sqlc.arg(group_id) AS INTEGER))
    OR EXISTS (SELECT 1 FROM invoice_payments ip WHERE ip.card_id = sqlc.arg(card_id) AND coalesce(ip.group_id, 0) <> CAST(sqlc.arg(group_id) AS INTEGER)) AS INTEGER) AS used;
