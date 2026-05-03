-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `account` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `provider`       VARCHAR(32) NOT NULL                COMMENT 'gpt / grok',
  `name`           VARCHAR(128) NOT NULL,
  `auth_type`      VARCHAR(32) NOT NULL                COMMENT 'api_key / cookie / oauth',
  `credential_enc` BLOB NOT NULL                        COMMENT 'AES-256-GCM 加密',
  `base_url`       VARCHAR(255) DEFAULT NULL,
  `model_whitelist` JSON DEFAULT NULL,
  `weight`         INT NOT NULL DEFAULT 10,
  `rpm_limit`      INT NOT NULL DEFAULT 0,
  `tpm_limit`      INT NOT NULL DEFAULT 0,
  `daily_quota`    INT NOT NULL DEFAULT 0,
  `monthly_quota`  INT NOT NULL DEFAULT 0,
  `status`         TINYINT NOT NULL DEFAULT 1          COMMENT '1启用 0停用 2熔断 -1禁用',
  `cooldown_until` DATETIME(3) DEFAULT NULL,
  `last_used_at`   DATETIME(3) DEFAULT NULL,
  `last_error`     VARCHAR(255) DEFAULT NULL,
  `error_count`    INT NOT NULL DEFAULT 0,
  `success_count`  BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `remark`         VARCHAR(255) DEFAULT NULL,
  `created_by`     BIGINT UNSIGNED DEFAULT NULL,
  `created_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`     DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_provider_status` (`provider`, `status`),
  KEY `idx_status_cooldown` (`status`, `cooldown_until`),
  KEY `idx_deleted` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='第三方账号池';

CREATE TABLE IF NOT EXISTS `account_group` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `provider`   VARCHAR(32) NOT NULL,
  `code`       VARCHAR(64) NOT NULL,
  `name`       VARCHAR(128) NOT NULL,
  `strategy`   VARCHAR(32) NOT NULL DEFAULT 'round_robin',
  `status`     TINYINT NOT NULL DEFAULT 1,
  `remark`     VARCHAR(255) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_provider_code` (`provider`, `code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='账号池分组';

CREATE TABLE IF NOT EXISTS `account_group_member` (
  `group_id`   BIGINT UNSIGNED NOT NULL,
  `account_id` BIGINT UNSIGNED NOT NULL,
  `weight`     INT NOT NULL DEFAULT 10,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`group_id`, `account_id`),
  KEY `idx_account` (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='账号-分组成员';

CREATE TABLE IF NOT EXISTS `account_health` (
  `account_id`        BIGINT UNSIGNED NOT NULL,
  `last_check_at`     DATETIME(3) NOT NULL,
  `last_check_status` TINYINT NOT NULL,
  `consec_fail`       INT NOT NULL DEFAULT 0,
  `latency_ms_p50`    INT NOT NULL DEFAULT 0,
  `latency_ms_p99`    INT NOT NULL DEFAULT 0,
  `error_rate_1h`     DECIMAL(5,2) NOT NULL DEFAULT 0.00,
  `updated_at`        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='账号健康指标';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `account_health`;
DROP TABLE IF EXISTS `account_group_member`;
DROP TABLE IF EXISTS `account_group`;
DROP TABLE IF EXISTS `account`;
