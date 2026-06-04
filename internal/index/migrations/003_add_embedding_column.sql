-- +goose Up
ALTER TABLE notes ADD COLUMN embedding BLOB;
