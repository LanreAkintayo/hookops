-- Remove rate_limit column from endpoints table.
ALTER TABLE endpoints 
DROP COLUMN IF EXISTS rate_limit;
