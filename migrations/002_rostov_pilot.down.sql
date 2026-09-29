DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS partner_sessions;
DROP TABLE IF EXISTS dialog_states;
DROP TABLE IF EXISTS waitlist;
DROP INDEX IF EXISTS uq_bookings_active_checkin;
DROP INDEX IF EXISTS uq_bookings_active_user_slot;
ALTER TABLE bookings
    DROP COLUMN IF EXISTS rating,
    DROP COLUMN IF EXISTS feedback_asked,
    DROP COLUMN IF EXISTS reminder_2h_sent,
    DROP COLUMN IF EXISTS reminder_24h_sent,
    DROP COLUMN IF EXISTS attended_at,
    DROP COLUMN IF EXISTS checkin_code;
ALTER TABLE slots DROP COLUMN IF EXISTS level, DROP COLUMN IF EXISTS sport_type, DROP COLUMN IF EXISTS title;
DROP INDEX IF EXISTS idx_venues_sports;
DROP INDEX IF EXISTS uq_venues_external_id;
ALTER TABLE venues
    DROP COLUMN IF EXISTS verified_at, DROP COLUMN IF EXISTS trial_price,
    DROP COLUMN IF EXISTS what_to_bring, DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS opening_hours, DROP COLUMN IF EXISTS website,
    DROP COLUMN IF EXISTS phone, DROP COLUMN IF EXISTS booking_mode,
    DROP COLUMN IF EXISTS source_url, DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS district, DROP COLUMN IF EXISTS sports,
    DROP COLUMN IF EXISTS external_id;
