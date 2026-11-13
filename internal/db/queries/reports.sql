-- name: CardSpendingByMonth :many
-- Couple share of card installments and credits by invoice month and category (RN-01, RN-04).
SELECT i.invoice_month AS month, coalesce(p.category_id, 0) AS category_id,
       CAST(sum(i.amount_cents - coalesce((
           SELECT sum(si.amount_cents) FROM split_installments si JOIN splits s ON s.id = si.split_id
           WHERE s.purchase_id = i.purchase_id AND si.number = i.number
       ), 0)) AS INTEGER) AS cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND i.invoice_month >= sqlc.arg(first_month) AND i.invoice_month <= sqlc.arg(last_month)
GROUP BY i.invoice_month, p.category_id;

-- name: EntrySpendingByMonth :many
-- Confirmed expenses paid from accounts by month of the date and category.
SELECT substr(occurred_on, 1, 7) AS month, coalesce(category_id, 0) AS category_id, CAST(-sum(amount_cents) AS INTEGER) AS cents
FROM entries
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND kind = 'expense' AND confirmed AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day)
GROUP BY substr(occurred_on, 1, 7), category_id;

-- name: AleloSpendingByMonth :many
-- Alelo purchases by month of the date, always groceries (RN-15).
SELECT substr(purchased_on, 1, 7) AS month, category_id, CAST(sum(amount_cents) AS INTEGER) AS cents
FROM alelo_purchases
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND purchased_on >= sqlc.arg(first_day) AND purchased_on <= sqlc.arg(last_day)
GROUP BY substr(purchased_on, 1, 7), category_id;

-- name: ListBudgets :many
SELECT category_id, amount_cents
FROM budgets
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER);

-- name: UpsertBudget :exec
INSERT INTO budgets (category_id, amount_cents, created_by, updated_at, group_id)
VALUES (?, ?, ?, ?, CAST(sqlc.arg(group_id) AS INTEGER))
ON CONFLICT (group_id, category_id) DO UPDATE SET amount_cents = excluded.amount_cents, updated_at = excluded.updated_at;

-- name: DeleteBudget :exec
DELETE FROM budgets
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND category_id = sqlc.arg(category_id);

-- name: DayToDaySpending :one
-- One off expenses paid from accounts in the period, without bill and financing payments.
SELECT CAST(coalesce(-sum(e.amount_cents), 0) AS INTEGER) AS cents
FROM entries e
WHERE e.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND e.kind = 'expense' AND e.confirmed AND e.bill_id IS NULL
  AND e.id NOT IN (SELECT entry_id FROM financing_events WHERE entry_id IS NOT NULL)
  AND e.occurred_on >= sqlc.arg(first_day) AND e.occurred_on <= sqlc.arg(last_day);

-- name: CardInstallmentsByMonth :many
-- Couple share of card installments by invoice month, without recurrences (RF-60).
SELECT i.invoice_month AS month,
       CAST(sum(i.amount_cents - coalesce((
           SELECT sum(si.amount_cents) FROM split_installments si JOIN splits s ON s.id = si.split_id
           WHERE s.purchase_id = i.purchase_id AND si.number = i.number
       ), 0)) AS INTEGER) AS cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND i.invoice_month >= sqlc.arg(first_month) AND i.invoice_month <= sqlc.arg(last_month)
GROUP BY i.invoice_month;

-- name: AccountsTotal :one
-- Real balance of the accounts of the members: initial balances plus every confirmed movement, in any space.
-- Only the personal space adds it to the net worth (RF-62, RF-83).
SELECT CAST(coalesce(sum(a.initial_balance_cents), 0) + coalesce((
           SELECT sum(e.amount_cents) FROM entries e JOIN accounts ea ON ea.id = e.account_id
           WHERE e.confirmed
             AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = ea.person_id)
       ), 0) AS INTEGER) AS cents
FROM accounts a
WHERE EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = a.person_id);

-- name: SavedInPeriod :one
-- Deposits minus withdrawals of the savings boxes in the period, by date (RF-61).
SELECT CAST(coalesce(-sum(amount_cents), 0) AS INTEGER) AS cents
FROM entries
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND kind IN ('savings_deposit', 'savings_withdrawal') AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day);

-- name: ReceivedInPeriod :one
-- Shares paid back by other people in the period, by date (RF-61).
SELECT CAST(coalesce(sum(amount_cents), 0) AS INTEGER) AS cents
FROM entries
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND kind = 'receivable' AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day);

-- name: IncomeInPeriod :one
-- Confirmed income that fell in the period, by date, like the base of the savings (RN-09).
SELECT CAST(coalesce(sum(amount_cents), 0) AS INTEGER) AS cents
FROM entries
WHERE group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND kind = 'income' AND confirmed AND occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day);

-- name: SearchEntries :many
-- Every entry, card purchase and Alelo purchase in the period, filtered (RF-63).
-- Zero and empty filters are off; a top level category matches its subcategories too.
SELECT CAST(source AS TEXT) AS source, CAST(id AS INTEGER) AS id, CAST(occurred_on AS TEXT) AS occurred_on,
       CAST(description AS TEXT) AS description, CAST(amount_cents AS INTEGER) AS amount_cents, CAST(kind AS TEXT) AS kind,
       CAST(category_id AS INTEGER) AS category_id, CAST(category_name AS TEXT) AS category_name,
       CAST(parent_category_name AS TEXT) AS parent_category_name, CAST(account_id AS INTEGER) AS account_id,
       CAST(account_name AS TEXT) AS account_name, CAST(card_id AS INTEGER) AS card_id, CAST(card_name AS TEXT) AS card_name,
       CAST(person_id AS INTEGER) AS person_id, CAST(person_name AS TEXT) AS person_name
FROM (
    SELECT 'account' AS source, e.id, e.occurred_on, e.description, e.amount_cents, e.kind,
           coalesce(e.category_id, 0) AS category_id, coalesce(c.name, '') AS category_name, coalesce(pc.name, '') AS parent_category_name,
           coalesce(c.parent_id, 0) AS category_parent_id, a.id AS account_id, a.name AS account_name, 0 AS card_id, '' AS card_name,
           pe.id AS person_id, pe.name AS person_name
    FROM entries e
    JOIN accounts a ON a.id = e.account_id
    LEFT JOIN categories c ON c.id = e.category_id
    LEFT JOIN categories pc ON pc.id = c.parent_id
    JOIN users u ON u.id = e.created_by
    JOIN persons pe ON pe.id = u.person_id
    WHERE e.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND e.confirmed
    UNION ALL
    SELECT 'card', p.id, p.purchased_on, p.merchant, CASE WHEN p.kind = 'credit' THEN p.total_cents ELSE -p.total_cents END, p.kind,
           coalesce(p.category_id, 0), coalesce(c.name, ''), coalesce(pc.name, ''), coalesce(c.parent_id, 0),
           0, '', cd.id, cd.name, pe.id, pe.name
    FROM purchases p
    JOIN cards cd ON cd.id = p.card_id
    LEFT JOIN categories c ON c.id = p.category_id
    LEFT JOIN categories pc ON pc.id = c.parent_id
    JOIN users u ON u.id = p.created_by
    JOIN persons pe ON pe.id = coalesce(p.buyer_person_id, u.person_id)
    WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
    UNION ALL
    SELECT 'alelo', ap.id, ap.purchased_on, ap.description, -ap.amount_cents, 'alelo',
           ap.category_id, coalesce(c.name, ''), coalesce(pc.name, ''), coalesce(c.parent_id, 0),
           0, '', 0, al.name, pe.id, pe.name
    FROM alelo_purchases ap
    JOIN alelo_cards al ON al.id = ap.alelo_id
    LEFT JOIN categories c ON c.id = ap.category_id
    LEFT JOIN categories pc ON pc.id = c.parent_id
    JOIN users u ON u.id = ap.created_by
    JOIN persons pe ON pe.id = u.person_id
    WHERE ap.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
) AS x
WHERE occurred_on >= sqlc.arg(first_day) AND occurred_on <= sqlc.arg(last_day)
  AND (CAST(sqlc.arg(category_id) AS INTEGER) = 0 OR category_id = CAST(sqlc.arg(category_id) AS INTEGER) OR category_parent_id = CAST(sqlc.arg(category_id) AS INTEGER))
  AND (CAST(sqlc.arg(account_id) AS INTEGER) = 0 OR account_id = CAST(sqlc.arg(account_id) AS INTEGER))
  AND (CAST(sqlc.arg(card_id) AS INTEGER) = 0 OR (source = 'card' AND card_id = CAST(sqlc.arg(card_id) AS INTEGER)))
  AND (CAST(sqlc.arg(person_id) AS INTEGER) = 0 OR person_id = CAST(sqlc.arg(person_id) AS INTEGER))
  AND (CAST(sqlc.arg(text) AS TEXT) = '' OR instr(lower(description), lower(CAST(sqlc.arg(text) AS TEXT))) > 0)
ORDER BY occurred_on DESC, source, id DESC
LIMIT 500;
