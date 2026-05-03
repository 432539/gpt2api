-- +goose Up
-- +goose StatementBegin

ALTER TABLE `account`
  ADD COLUMN `session_token_enc` BLOB DEFAULT NULL COMMENT 'AES-GCM session_token（如 ST / id_token 存证）' AFTER `refresh_token_enc`;

-- +goose StatementEnd

-- +goose Down
ALTER TABLE `account` DROP COLUMN `session_token_enc`;
