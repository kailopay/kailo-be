ALTER TABLE stellar_transactions
    ADD COLUMN asset_issuer text NOT NULL DEFAULT '';
