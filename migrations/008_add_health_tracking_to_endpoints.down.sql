-- Remove consecutive_failures column from endpoints table.
ALTER TABLE endpoints 
DROP COLUMN IF EXISTS consecutive_failures;
