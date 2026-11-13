-- name: DeleteMemberSplit :exec
DELETE FROM member_splits
WHERE item_kind = sqlc.arg(item_kind) AND item_id = sqlc.arg(item_id);

-- name: InsertMemberSplit :exec
INSERT INTO member_splits (item_kind, item_id, mode, person_id, share_cents)
VALUES (sqlc.arg(item_kind), sqlc.arg(item_id), sqlc.arg(mode), sqlc.arg(person_id), sqlc.arg(share_cents));

-- name: ListMemberSplit :many
SELECT mode, person_id, share_cents
FROM member_splits
WHERE item_kind = sqlc.arg(item_kind) AND item_id = sqlc.arg(item_id)
ORDER BY person_id;

-- name: ListGroupMemberSplits :many
-- Every division of the items of a group, read once for the settlement (RN-28).
SELECT ms.item_kind, ms.item_id, ms.mode, ms.person_id, ms.share_cents
FROM member_splits ms
WHERE (ms.item_kind = 'entry' AND EXISTS (SELECT 1 FROM entries x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
   OR (ms.item_kind = 'purchase' AND EXISTS (SELECT 1 FROM purchases x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
   OR (ms.item_kind = 'recurrence' AND EXISTS (SELECT 1 FROM recurrences x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
   OR (ms.item_kind = 'bill' AND EXISTS (SELECT 1 FROM bills x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
   OR (ms.item_kind = 'financing' AND EXISTS (SELECT 1 FROM financings x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
   OR (ms.item_kind = 'alelo' AND EXISTS (SELECT 1 FROM alelo_purchases x WHERE x.id = ms.item_id AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER)))
ORDER BY ms.item_kind, ms.item_id, ms.person_id;

-- name: ListGroupWeights :many
-- Weights of the members over time, oldest first (RN-28).
SELECT w.person_id, w.weight_bp, w.from_day
FROM group_weights w
WHERE w.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY w.from_day, w.person_id;

-- name: UpsertGroupWeight :exec
INSERT INTO group_weights (group_id, person_id, weight_bp, from_day)
VALUES (CAST(sqlc.arg(group_id) AS INTEGER), sqlc.arg(person_id), sqlc.arg(weight_bp), sqlc.arg(from_day))
ON CONFLICT (group_id, person_id, from_day) DO UPDATE SET weight_bp = excluded.weight_bp;

-- name: SetGroupSettlesFrom :exec
UPDATE groups
SET settles_from = sqlc.arg(settles_from)
WHERE id = sqlc.arg(id);

-- name: GroupSettlesFrom :one
SELECT CAST(coalesce(settles_from, '') AS TEXT) AS settles_from
FROM groups
WHERE id = sqlc.arg(id);

-- name: SettlementExpenses :many
-- Confirmed expenses of accounts in the group from the start of the settlement, with who holds the account and the
-- division that applies: the bill or the financing they pay, or the entry itself (RF-84).
SELECT e.id, e.occurred_on, e.amount_cents, e.description, a.person_id AS payer_id, a.name AS account_name,
       coalesce(c.name, '') AS category_name,
       CAST(coalesce(e.bill_id, 0) AS INTEGER) AS bill_id,
       CAST(coalesce((SELECT fe.financing_id FROM financing_events fe WHERE fe.entry_id = e.id), 0) AS INTEGER) AS financing_id
FROM entries e
JOIN accounts a ON a.id = e.account_id
LEFT JOIN categories c ON c.id = e.category_id
WHERE e.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND e.kind = 'expense' AND e.confirmed
  AND e.occurred_on >= sqlc.arg(from_day) AND e.occurred_on <= sqlc.arg(to_day)
ORDER BY e.occurred_on, e.id;

-- name: SettlementAleloPurchases :many
-- Alelo purchases of the group from the start of the settlement, paid by who holds the card (RF-84).
SELECT ap.id, ap.purchased_on, ap.amount_cents, ap.description, ac.person_id AS payer_id, ac.name AS card_name
FROM alelo_purchases ap
JOIN alelo_cards ac ON ac.id = ap.alelo_id
WHERE ap.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND ap.purchased_on >= sqlc.arg(from_day) AND ap.purchased_on <= sqlc.arg(to_day)
ORDER BY ap.purchased_on, ap.id;

-- name: GroupInstallments :many
-- Installments of the group's purchases on a card with the part of other people in each, for the settlement (RF-84).
SELECT i.id, i.purchase_id, i.invoice_month, i.number, i.amount_cents, p.merchant, p.installment_count,
       CAST(coalesce((SELECT sum(si.amount_cents) FROM split_installments si JOIN splits s ON s.id = si.split_id
                      WHERE s.purchase_id = p.id AND si.number = i.number), 0) AS INTEGER) AS third_party_cents
FROM installments i
JOIN purchases p ON p.id = i.purchase_id
WHERE p.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND i.card_id = sqlc.arg(card_id)
ORDER BY i.invoice_month, i.id;

-- name: CreateSettlement :one
INSERT INTO settlements (group_id, transfer_id, from_person_id, to_person_id, amount_cents, paid_on, created_by, created_at)
VALUES (CAST(sqlc.arg(group_id) AS INTEGER), sqlc.arg(transfer_id), sqlc.arg(from_person_id), sqlc.arg(to_person_id),
        sqlc.arg(amount_cents), sqlc.arg(paid_on), sqlc.arg(created_by), sqlc.arg(created_at))
RETURNING id;

-- name: ListSettlements :many
SELECT s.id, s.transfer_id, s.from_person_id, s.to_person_id, s.amount_cents, s.paid_on,
       fp.name AS from_name, tp.name AS to_name
FROM settlements s
JOIN persons fp ON fp.id = s.from_person_id
JOIN persons tp ON tp.id = s.to_person_id
WHERE s.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
ORDER BY s.paid_on DESC, s.id DESC;

-- name: GetSettlement :one
SELECT id, transfer_id
FROM settlements
WHERE id = sqlc.arg(id) AND group_id = CAST(sqlc.arg(group_id) AS INTEGER);


-- name: SplitItemInGroup :one
-- Whether an item that may carry a division between members belongs to the group (RN-28).
SELECT CAST(CASE CAST(sqlc.arg(kind) AS TEXT)
    WHEN 'entry' THEN EXISTS (SELECT 1 FROM entries x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'purchase' THEN EXISTS (SELECT 1 FROM purchases x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'recurrence' THEN EXISTS (SELECT 1 FROM recurrences x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'bill' THEN EXISTS (SELECT 1 FROM bills x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'financing' THEN EXISTS (SELECT 1 FROM financings x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'alelo' THEN EXISTS (SELECT 1 FROM alelo_purchases x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    ELSE 0 END AS INTEGER) AS found;
