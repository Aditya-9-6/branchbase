-- MySQL Dialect migration
-- Alters column type and name
ALTER TABLE orders CHANGE COLUMN amount total_amount DECIMAL(10,2);
