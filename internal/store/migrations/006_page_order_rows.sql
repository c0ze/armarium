-- Each stored page number records the page numbering it was written in: 1
-- before v0.5.0, 2 for the root-first order. The remap moves only rows still
-- at 1, each once however often it runs, and never a row written since; new
-- rows default to the current numbering.
ALTER TABLE progress ADD COLUMN page_order INTEGER NOT NULL DEFAULT 2;
ALTER TABLE panel_correction ADD COLUMN page_order INTEGER NOT NULL DEFAULT 2;
-- Only comic archives' pages were renumbered, and a comic not started or
-- finished (its last page) means the same in both orders.
UPDATE progress SET page_order = 1 WHERE page > 0 AND status <> 'read'
  AND item_id IN (SELECT id FROM item WHERE format IN ('cbz', 'cbr'));
UPDATE panel_correction SET page_order = 1
  WHERE item_id IN (SELECT id FROM item WHERE format IN ('cbz', 'cbr'));
