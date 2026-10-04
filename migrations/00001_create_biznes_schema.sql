-- +goose Up
CREATE SCHEMA biznes;

-- +goose Down
DROP SCHEMA biznes RESTRICT;
