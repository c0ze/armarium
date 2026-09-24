-- Folded search text (see fold.go): computing fold() for every row on every
-- search tripled its cost, which the NAS CPU feels. Written on each upsert.
ALTER TABLE series ADD COLUMN search TEXT NOT NULL DEFAULT '';
UPDATE series SET search = fold(name);
ALTER TABLE item ADD COLUMN search TEXT NOT NULL DEFAULT '';
UPDATE item SET search = fold(title || char(10) || author || char(10) ||
                              (SELECT name FROM series WHERE series.id = item.series_id));
