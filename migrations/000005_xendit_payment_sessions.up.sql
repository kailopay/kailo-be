-- Payment Sessions use a hosted Xendit checkout. `xendit` means that the
-- customer may choose any payment channel enabled for the merchant account.
ALTER TABLE orders DROP CONSTRAINT orders_payment_method_check;
ALTER TABLE orders ADD CONSTRAINT orders_payment_method_check
    CHECK (payment_method IN ('qris', 'bri_va', 'xendit'));

ALTER TABLE payment_checkouts DROP CONSTRAINT payment_checkouts_method_check;
ALTER TABLE payment_checkouts ADD CONSTRAINT payment_checkouts_method_check
    CHECK (method IN ('qris', 'bri_va', 'xendit'));
