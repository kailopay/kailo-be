DROP INDEX IF EXISTS idx_kyc_inquiries_user_created;
CREATE INDEX idx_kyc_inquiries_user_updated
    ON kyc_inquiries(user_id, updated_at DESC, id DESC);
