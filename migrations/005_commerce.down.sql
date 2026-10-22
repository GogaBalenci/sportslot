DROP TABLE IF EXISTS ops_status;
DROP TABLE IF EXISTS digest_log;
ALTER TABLE studio_messages DROP COLUMN IF EXISTS studio_nudged_at, DROP COLUMN IF EXISTS client_notified_at;
DROP TABLE IF EXISTS payments;
DROP INDEX IF EXISTS idx_bookings_host;
DROP INDEX IF EXISTS uq_bookings_guest_token;
DELETE FROM bookings WHERE guest_token IS NOT NULL;
DROP INDEX IF EXISTS uq_bookings_active_user_slot;
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_active_user_slot
    ON bookings(user_id, slot_id) WHERE status = 'confirmed';
ALTER TABLE bookings
    DROP COLUMN IF EXISTS paid_at,
    DROP COLUMN IF EXISTS user_pass_id,
    DROP COLUMN IF EXISTS host_booking_id,
    DROP COLUMN IF EXISTS guest_token,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS review;
DROP TABLE IF EXISTS user_passes;
DROP TABLE IF EXISTS passes;
DROP TABLE IF EXISTS studio_settings;
