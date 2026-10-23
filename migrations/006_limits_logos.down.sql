DROP INDEX IF EXISTS idx_user_passes_created;
DROP INDEX IF EXISTS idx_messages_client;
DROP INDEX IF EXISTS idx_bookings_user_created;
DROP TABLE IF EXISTS venue_logos;
ALTER TABLE venues DROP COLUMN IF EXISTS profile_edited, DROP COLUMN IF EXISTS logo_version;
