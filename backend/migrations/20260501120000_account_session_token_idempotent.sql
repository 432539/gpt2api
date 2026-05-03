-- +goose Up
-- +goose StatementBegin
-- 幂等：已存在 session_token_enc 时跳过。适用于早期数据卷未执行 20260428140000 的情况。
SET @__klein_stmt := (
  SELECT IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
      WHERE TABLE_SCHEMA = DATABASE()
        AND TABLE_NAME = 'account'
        AND COLUMN_NAME = 'session_token_enc') < 1,
    'ALTER TABLE `account` ADD COLUMN `session_token_enc` BLOB DEFAULT NULL COMMENT ''AES-GCM session / id_token''',
    'SELECT 1'
  )
);
PREPARE __klein_prep FROM @__klein_stmt;
EXECUTE __klein_prep;
DEALLOCATE PREPARE __klein_prep;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SET @__klein_stmt := (
  SELECT IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
      WHERE TABLE_SCHEMA = DATABASE()
        AND TABLE_NAME = 'account'
        AND COLUMN_NAME = 'session_token_enc') > 0,
    'ALTER TABLE `account` DROP COLUMN `session_token_enc`',
    'SELECT 1'
  )
);
PREPARE __klein_prep FROM @__klein_stmt;
EXECUTE __klein_prep;
DEALLOCATE PREPARE __klein_prep;
-- +goose StatementEnd
