DROP TABLE IF EXISTS studio_messages;
DROP TABLE IF EXISTS studio_invites;
DROP TABLE IF EXISTS studio_admins;
ALTER TABLE bookings DROP COLUMN IF EXISTS cancel_reason;
DROP INDEX IF EXISTS idx_slots_template;
ALTER TABLE slots DROP COLUMN IF EXISTS cancelled_at, DROP COLUMN IF EXISTS booking_closed, DROP COLUMN IF EXISTS template_id;
DROP TABLE IF EXISTS schedule_templates;
ALTER TABLE venues DROP CONSTRAINT IF EXISTS venues_source_check;
ALTER TABLE venues ADD CONSTRAINT venues_source_check CHECK (source IN ('osm', 'demo_partner'));
