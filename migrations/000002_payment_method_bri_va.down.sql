ALTER TABLE payment_checkouts
    DROP CONSTRAINT payment_checkouts_method_check,
    ADD CONSTRAINT payment_checkouts_method_check CHECK (method IN ('qris', 'bank_transfer'));

ALTER TABLE orders
    DROP CONSTRAINT orders_payment_method_check,
    ADD CONSTRAINT orders_payment_method_check CHECK (payment_method IN ('qris', 'bank_transfer'));
