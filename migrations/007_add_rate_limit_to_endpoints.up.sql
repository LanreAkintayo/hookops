-- Add rate_limit column to endpoints table with a default of 10 requests/second
-- and a check constraint ensuring rate_limit is between 1 and 1000 requests/second.
ALTER TABLE endpoints 
ADD COLUMN IF NOT EXISTS rate_limit INTEGER NOT NULL DEFAULT 10 
CHECK (rate_limit > 0 AND rate_limit <= 1000);
