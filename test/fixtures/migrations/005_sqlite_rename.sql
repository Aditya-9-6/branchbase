-- SQLite Dialect migration
-- Renames column
ALTER TABLE items RENAME COLUMN sku TO item_sku;
