-- +goose Up
CREATE TABLE IF NOT EXISTS __TABLE_FULL__ (
    ts DateTime64(3),
    host String,
    labels Map(String, String),
    message String
) ENGINE = MergeTree ORDER BY (host, ts);
-- +goose Down
DROP TABLE IF EXISTS __TABLE_FULL__;
