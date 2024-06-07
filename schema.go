package main

const dbSchema = `
CREATE TABLE IF NOT EXISTS message_ref (
    id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    message_id INTEGER NOT NULL,
    chat_id INTEGER NOT NULL,
    last_used DATETIME
);

CREATE TABLE IF NOT EXISTS user (
    user_id INTEGER NOT NULL PRIMARY KEY,
    last_used DATETIME
);

CREATE TABLE IF NOT EXISTS gpt_hist (
    id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    user_name TEXT NOT NULL,
    user_msg TEXT NOT NULL,
    bot_msg TEXT NOT NULL,
    last_used DATETIME
)`
