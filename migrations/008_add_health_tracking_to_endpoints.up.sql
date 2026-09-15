-- Add consecutive_failures column to endpoints table to track delivery failure streaks.
ALTER TABLE endpoints 
ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER NOT NULL DEFAULT 0;
