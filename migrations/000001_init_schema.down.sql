-- +migrate Down
-- SQL in this section is executed when the migration is rolled back.

-- Drop indexes for messages table
DROP INDEX IF EXISTS idx_messages_chat_id;
DROP INDEX IF EXISTS idx_messages_user_id;
DROP INDEX IF EXISTS idx_messages_timestamp;
DROP INDEX IF EXISTS idx_messages_processed_at;
DROP TABLE IF EXISTS messages;

-- Drop indexes for user_profiles table
DROP INDEX IF EXISTS idx_user_profiles_user_id;
DROP TABLE IF EXISTS user_profiles;
