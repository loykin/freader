-- +goose Up
CREATE TABLE offsets (
                         id TEXT NOT NULL,
                         strategy TEXT NOT NULL,
                         path TEXT NOT NULL,
                         offset BIGINT NOT NULL,
                         created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                         PRIMARY KEY (id, strategy)
);

CREATE INDEX idx_offsets_path ON offsets(path);

-- +goose Down
DROP TABLE offsets;