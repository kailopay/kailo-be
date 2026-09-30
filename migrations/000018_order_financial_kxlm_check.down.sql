ALTER TABLE order_financials
    DROP CONSTRAINT order_financials_asset_code_check;

ALTER TABLE order_financials
    ADD CONSTRAINT order_financials_asset_code_check
    CHECK (asset_code = 'XLM');
