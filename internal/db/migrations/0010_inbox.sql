-- Inbox: track when the human last reviewed the AI activity feed, so the
-- inbox can surface "what's new since you were away" + a sidebar badge.
ALTER TABLE users ADD COLUMN IF NOT EXISTS inbox_seen_at timestamptz;
