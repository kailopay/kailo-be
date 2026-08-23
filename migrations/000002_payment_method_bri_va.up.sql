-- Align the stored on-ramp payment method values with the public API enum.
-- The public contract and the Xendit channel mapping use `bri_va`, while
-- migration 000001 only allowed the unused `bank_transfer` label.

ALTER TABLE orders
    DROP CONSTRAINT orders_payment_method_check,
    ADD CONSTRAINT orders_payment_method_check CHECK (payment_method IN ('qris', 'bri_va'));

ALTER TABLE payment_checkouts
    DROP CONSTRAINT payment_checkouts_method_check,
    ADD CONSTRAINT payment_checkouts_method_check CHECK (method IN ('qris', 'bri_va'));
