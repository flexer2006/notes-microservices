DROP TRIGGER IF EXISTS set_timestamp ON notes;
DROP FUNCTION IF EXISTS trigger_set_timestamp();
DROP INDEX IF EXISTS idx_notes_user_id;
DROP TABLE IF EXISTS notes;