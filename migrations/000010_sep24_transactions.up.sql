CREATE TABLE sep24_transactions (
    id uuid PRIMARY KEY,
    transaction_id text NOT NULL UNIQUE CHECK (char_length(transaction_id) BETWEEN 1 AND 255),
    order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
    kind text NOT NULL CHECK (kind IN ('deposit', 'withdraw')),
    created_at timestamptz NOT NULL
);
