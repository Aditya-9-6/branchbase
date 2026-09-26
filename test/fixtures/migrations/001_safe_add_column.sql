-- Safe additive migration
-- Adds a nullable column without locking existing transactions
ALTER TABLE users ADD COLUMN bio TEXT;
