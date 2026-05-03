-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `api_key` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`      BIGINT UNSIGNED NOT NULL,
  `name`         VARCHAR(64) NOT NULL,
  `prefix`       VARCHAR(16) NOT NULL,
  `hash`         CHAR(64) NOT NULL                   COMMENT 'SHA256(key + salt)',
  `salt`         CHAR(32) NOT NULL,
  `last4`        CHAR(4) NOT NULL,
  `scope`        VARCHAR(255) NOT NULL DEFAULT 'image,video',
  `rpm_limit`    INT NOT NULL DEFAULT 60,
  `daily_quota`  INT NOT NULL DEFAULT 0,
  `expire_at`    DATETIME(3) DEFAULT NULL,
  `last_used_at` DATETIME(3) DEFAULT NULL,
  `status`       TINYINT NOT NULL DEFAULT 1,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`   DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_hash` (`hash`),
  KEY `idx_user_status` (`user_id`, `status`),
  KEY `idx_deleted` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户 API Key';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `api_key`;
