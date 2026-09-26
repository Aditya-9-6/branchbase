-- Destructive breaking migration
-- Drops a column that has inbound Foreign Keys in orders and payments
ALTER TABLE users DROP COLUMN email;
