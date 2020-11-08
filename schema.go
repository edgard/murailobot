package main

var schema = `
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id INTEGER,
    chat_id INTEGER,
    last_used INTEGER
);`
