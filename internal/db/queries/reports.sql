-- name: CardSpendingByMonth :many
-- Couple share of card installments and credits by invoice month and category (RN-01, RN-04).
SELECT i.invoice_month AS month, coalesce(p.category_id, 0) AS category_id,
       CAST(sum(i.amount_cents - coalesce((
           SELECT sum(si.amount_cents) FROM split_installments si JOIN splits s ON s.id = si.split_id
           WHERE s.purchase_id = i.purchase_id AND si.number = i.number
       ), 0)) AS INTEGER) AS cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE i.invoice_month >= sqlc.arg(first_month) AND i.invoice_month <= sqlc.arg(last_month)
GROUP BY i.invoice_month, p.category_id;

-- name: EntrySpendingByMonth :many
-- Confirmed expenses paid from accounts by month of the date and category.
SELECT substr(occurred_on, 1, 7) AS month, coalesce(category_id, 0) AS category_id, CAST(-sum(amount_cents) AS INTEGER) AS cents
FROM entries
WHERE kind = 'expense' AND confirmed AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day)
GROUP BY substr(occurred_on, 1, 7), category_id;

-- name: AleloSpendingByMonth :many
-- Alelo purchases by month of the date, always groceries (RN-15).
SELECT substr(purchased_on, 1, 7) AS month, category_id, CAST(sum(amount_cents) AS INTEGER) AS cents
FROM alelo_purchases
WHERE purchased_on >= sqlc.arg(first_day) AND purchased_on <= sqlc.arg(last_day)
GROUP BY substr(purchased_on, 1, 7), category_id;
