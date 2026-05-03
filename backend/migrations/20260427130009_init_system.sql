-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `system_config` (
  `key`        VARCHAR(64) NOT NULL,
  `value`      JSON NOT NULL,
  `remark`     VARCHAR(255) DEFAULT NULL,
  `updated_by` BIGINT UNSIGNED DEFAULT NULL,
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='系统全局配置';

CREATE TABLE IF NOT EXISTS `system_dict` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `dict_group` VARCHAR(64) NOT NULL,
  `dict_key`   VARCHAR(64) NOT NULL,
  `dict_value` VARCHAR(255) NOT NULL,
  `sort`       INT NOT NULL DEFAULT 0,
  `status`     TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_group_key` (`dict_group`, `dict_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='系统字典';

CREATE TABLE IF NOT EXISTS `announcement` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `title`      VARCHAR(128) NOT NULL,
  `content`    TEXT NOT NULL,
  `level`      VARCHAR(16) NOT NULL DEFAULT 'info',
  `start_at`   DATETIME(3) NOT NULL,
  `end_at`     DATETIME(3) NOT NULL,
  `status`     TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_status_time` (`status`, `start_at`, `end_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='公告';

CREATE TABLE IF NOT EXISTS `request_log` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `trace_id`     CHAR(36) NOT NULL,
  `user_id`      BIGINT UNSIGNED DEFAULT NULL,
  `api_key_id`   BIGINT UNSIGNED DEFAULT NULL,
  `method`       VARCHAR(8) NOT NULL,
  `path`         VARCHAR(255) NOT NULL,
  `status`       INT NOT NULL,
  `latency_ms`   INT NOT NULL,
  `client_ip`    VARCHAR(45) DEFAULT NULL,
  `ua`           VARCHAR(255) DEFAULT NULL,
  `req_size`     INT DEFAULT NULL,
  `resp_size`    INT DEFAULT NULL,
  `err_code`     INT DEFAULT NULL,
  `created_at`   DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`, `created_at`),
  KEY `idx_user_created` (`user_id`, `created_at`),
  KEY `idx_trace` (`trace_id`),
  KEY `idx_status_created` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='请求日志（按月分区）';

CREATE TABLE IF NOT EXISTS `pool_call_log` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`    CHAR(26) NOT NULL,
  `account_id` BIGINT UNSIGNED NOT NULL,
  `provider`   VARCHAR(32) NOT NULL,
  `endpoint`   VARCHAR(128) NOT NULL,
  `status`     INT NOT NULL,
  `latency_ms` INT NOT NULL,
  `tokens`     INT DEFAULT NULL,
  `error`      VARCHAR(255) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_task` (`task_id`),
  KEY `idx_account_created` (`account_id`, `created_at`),
  KEY `idx_status_created` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='账号池调用日志';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `pool_call_log`;
DROP TABLE IF EXISTS `request_log`;
DROP TABLE IF EXISTS `announcement`;
DROP TABLE IF EXISTS `system_dict`;
DROP TABLE IF EXISTS `system_config`;
