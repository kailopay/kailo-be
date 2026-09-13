DROP INDEX IF EXISTS idx_kyc_inquiries_user_updated;
CREATE INDEX idx_kyc_inquiries_user_created
    ON kyc_inquiries(user_id, created_at DESC, id DESC);
