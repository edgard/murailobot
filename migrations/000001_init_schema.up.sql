-- +migrate Up
-- SQL in this section is executed when the migration is applied.

-- Create messages table to store message history for context and analysis
CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL, -- Timestamp from Telegram
    processed_at TIMESTAMP NULL
);

-- Indexes for messages table
CREATE INDEX idx_messages_chat_id ON messages (chat_id);
CREATE INDEX idx_messages_user_id ON messages (user_id);
CREATE INDEX idx_messages_timestamp ON messages (timestamp);
CREATE INDEX idx_messages_processed_at ON messages (processed_at);


-- Create user_profiles table to store aggregated user information
CREATE TABLE user_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id INTEGER NOT NULL UNIQUE, -- Telegram User ID
    aliases TEXT,
    origin_location TEXT,
    current_location TEXT,
    age_range TEXT,
    traits TEXT
);

-- Indexes for user_profiles table
CREATE INDEX idx_user_profiles_user_id ON user_profiles (user_id);
