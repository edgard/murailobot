package main

var schema = `
CREATE TABLE IF NOT EXISTS messages (
    message_id integer PRIMARY KEY,
    chat_id integer
);`
