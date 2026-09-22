ALTER TABLE sep24_interactive_sessions
    ADD COLUMN IF NOT EXISTS browser_token_hash bytea;

UPDATE sep24_interactive_sessions
SET browser_token_hash = decode(md5(id::text), 'hex')
WHERE browser_token_hash IS NULL;

ALTER TABLE sep24_interactive_sessions
    ALTER COLUMN browser_token_hash SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_sep24_interactive_browser_token_hash
    ON sep24_interactive_sessions(browser_token_hash);
