DROP INDEX IF EXISTS idx_offramp_payouts_reference_id;
DROP INDEX IF EXISTS idx_sep24_transactions_external_id;
DROP INDEX IF EXISTS idx_sep24_transactions_stellar_id;
DROP INDEX IF EXISTS idx_sep24_transactions_wallet_created;

ALTER TABLE sep24_transactions
    DROP COLUMN IF EXISTS external_transaction_id,
    DROP COLUMN IF EXISTS stellar_transaction_id,
    DROP COLUMN IF EXISTS quote_id,
    DROP COLUMN IF EXISTS wallet_account;

DROP INDEX IF EXISTS idx_orders_quote_id;
DROP INDEX IF EXISTS idx_orders_wallet_created;
ALTER TABLE orders
    DROP COLUMN IF EXISTS quote_id,
    DROP COLUMN IF EXISTS wallet_account;

DROP INDEX IF EXISTS idx_sep24_interactive_user;
DROP INDEX IF EXISTS idx_sep24_interactive_wallet_expiry;
DROP TABLE IF EXISTS sep24_interactive_sessions;

DROP INDEX IF EXISTS idx_sep38_quotes_owner_expiry;
DROP TABLE IF EXISTS sep38_quotes;

DROP INDEX IF EXISTS idx_sep10_challenges_expiry;
DROP TABLE IF EXISTS sep10_challenges;
