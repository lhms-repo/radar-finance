-- Moving an item between the personal space and the groups of a person (RF-83). Every move checks that the
-- account, card or person of the item belongs to the target space, so the item never lands where its account
-- cannot be seen.

-- name: ItemInGroup :one
SELECT CAST(CASE CAST(sqlc.arg(kind) AS TEXT)
    WHEN 'entry' THEN EXISTS (SELECT 1 FROM entries x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'purchase' THEN EXISTS (SELECT 1 FROM purchases x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'bill' THEN EXISTS (SELECT 1 FROM bills x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'financing' THEN EXISTS (SELECT 1 FROM financings x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'income' THEN EXISTS (SELECT 1 FROM incomes x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'savings' THEN EXISTS (SELECT 1 FROM savings_boxes x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    WHEN 'alelo' THEN EXISTS (SELECT 1 FROM alelo_purchases x WHERE x.id = sqlc.arg(item_id) AND x.group_id = CAST(sqlc.arg(group_id) AS INTEGER))
    ELSE 0 END AS INTEGER) AS found;

-- name: MoveEntry :execrows
-- A one off expense or income, never one that pays a bill, an installment or confirms an income.
UPDATE entries
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE entries.id = sqlc.arg(item_id) AND entries.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND entries.kind IN ('expense', 'income') AND entries.income_id IS NULL AND entries.bill_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM financing_events fe WHERE fe.entry_id = entries.id)
  AND EXISTS (SELECT 1 FROM accounts a JOIN group_members gm ON gm.person_id = a.person_id
              WHERE a.id = entries.account_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MovePurchase :execrows
-- A card purchase with its installments; a purchase split with other people stays, since they are of the group.
-- It leaves its import, so undoing the import in the old space does not reach it.
UPDATE purchases
SET group_id = CAST(sqlc.arg(target) AS INTEGER), import_id = NULL
WHERE purchases.id = sqlc.arg(item_id) AND purchases.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND NOT EXISTS (SELECT 1 FROM splits s WHERE s.purchase_id = purchases.id)
  AND EXISTS (SELECT 1 FROM cards c JOIN group_members gm ON gm.person_id = c.person_id
              WHERE c.id = purchases.card_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MoveBill :execrows
UPDATE bills
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE bills.id = sqlc.arg(item_id) AND bills.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND EXISTS (SELECT 1 FROM accounts a JOIN group_members gm ON gm.person_id = a.person_id
              WHERE a.id = bills.account_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER))
  -- Every linked entry is on an account the target space sees.
  AND (SELECT count(*) FROM entries e JOIN accounts a ON a.id = e.account_id WHERE e.bill_id = bills.id)
    = (SELECT count(*) FROM entries e JOIN accounts a ON a.id = e.account_id JOIN group_members gm ON gm.person_id = a.person_id
       WHERE e.bill_id = bills.id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MoveBillEntries :exec
UPDATE entries
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE bill_id = sqlc.arg(item_id);

-- name: MoveFinancing :execrows
UPDATE financings
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE financings.id = sqlc.arg(item_id) AND financings.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND EXISTS (SELECT 1 FROM accounts a JOIN group_members gm ON gm.person_id = a.person_id
              WHERE a.id = financings.account_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER))
  -- Every linked entry is on an account the target space sees.
  AND (SELECT count(*) FROM financing_events fe JOIN entries e ON e.id = fe.entry_id JOIN accounts a ON a.id = e.account_id WHERE fe.financing_id = financings.id)
    = (SELECT count(*) FROM financing_events fe JOIN entries e ON e.id = fe.entry_id JOIN accounts a ON a.id = e.account_id JOIN group_members gm ON gm.person_id = a.person_id
       WHERE fe.financing_id = financings.id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MoveFinancingEntries :exec
UPDATE entries
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE id IN (SELECT fe.entry_id FROM financing_events fe WHERE fe.financing_id = sqlc.arg(item_id) AND fe.entry_id IS NOT NULL);

-- name: MoveIncome :execrows
-- The person who receives the income must also be in the target space.
UPDATE incomes
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE incomes.id = sqlc.arg(item_id) AND incomes.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND EXISTS (SELECT 1 FROM group_members gm WHERE gm.person_id = incomes.person_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER))
  AND EXISTS (SELECT 1 FROM accounts a JOIN group_members gm ON gm.person_id = a.person_id
              WHERE a.id = incomes.account_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER))
  -- Every linked entry is on an account the target space sees.
  AND (SELECT count(*) FROM entries e JOIN accounts a ON a.id = e.account_id WHERE e.income_id = incomes.id)
    = (SELECT count(*) FROM entries e JOIN accounts a ON a.id = e.account_id JOIN group_members gm ON gm.person_id = a.person_id
       WHERE e.income_id = incomes.id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MoveIncomeEntries :exec
UPDATE entries
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE income_id = sqlc.arg(item_id);

-- name: MoveSavingsBox :execrows
UPDATE savings_boxes
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE savings_boxes.id = sqlc.arg(item_id) AND savings_boxes.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  -- Every linked entry is on an account the target space sees.
  AND (SELECT count(*) FROM savings_moves sm JOIN entries e ON e.id = sm.entry_id JOIN accounts a ON a.id = e.account_id WHERE sm.box_id = savings_boxes.id)
    = (SELECT count(*) FROM savings_moves sm JOIN entries e ON e.id = sm.entry_id JOIN accounts a ON a.id = e.account_id JOIN group_members gm ON gm.person_id = a.person_id
       WHERE sm.box_id = savings_boxes.id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: MoveSavingsEntries :exec
UPDATE entries
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE id IN (SELECT sm.entry_id FROM savings_moves sm WHERE sm.box_id = sqlc.arg(item_id));

-- name: MoveAleloPurchase :execrows
-- A purchase on an Alelo card whose holder is in the target space; its division between members stays behind.
UPDATE alelo_purchases
SET group_id = CAST(sqlc.arg(target) AS INTEGER)
WHERE alelo_purchases.id = sqlc.arg(item_id) AND alelo_purchases.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
  AND EXISTS (SELECT 1 FROM alelo_cards c JOIN group_members gm ON gm.person_id = c.person_id
              WHERE c.id = alelo_purchases.alelo_id AND gm.group_id = CAST(sqlc.arg(target) AS INTEGER));

-- name: ItemCreator :one
-- Who recorded the item, the only one who moves it to another space (RF-83).
SELECT CAST(CASE CAST(sqlc.arg(kind) AS TEXT)
    WHEN 'entry' THEN (SELECT x.created_by FROM entries x WHERE x.id = sqlc.arg(item_id))
    WHEN 'purchase' THEN (SELECT x.created_by FROM purchases x WHERE x.id = sqlc.arg(item_id))
    WHEN 'bill' THEN (SELECT x.created_by FROM bills x WHERE x.id = sqlc.arg(item_id))
    WHEN 'financing' THEN (SELECT x.created_by FROM financings x WHERE x.id = sqlc.arg(item_id))
    WHEN 'income' THEN (SELECT x.created_by FROM incomes x WHERE x.id = sqlc.arg(item_id))
    WHEN 'savings' THEN (SELECT x.created_by FROM savings_boxes x WHERE x.id = sqlc.arg(item_id))
    WHEN 'alelo' THEN (SELECT x.created_by FROM alelo_purchases x WHERE x.id = sqlc.arg(item_id))
    ELSE 0 END AS INTEGER) AS created_by;
