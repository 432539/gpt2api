-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `model` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `code`         VARCHAR(64) NOT NULL,
  `name`         VARCHAR(128) NOT NULL,
  `kind`         VARCHAR(16) NOT NULL                COMMENT 'image / video',
  `provider`     VARCHAR(32) NOT NULL,
  `version`      VARCHAR(32) DEFAULT NULL,
  `tags`         VARCHAR(255) DEFAULT NULL,
  `cover_url`    VARCHAR(512) DEFAULT NULL,
  `description`  TEXT DEFAULT NULL,
  `point_per_unit` INT NOT NULL                      COMMENT '每张/每秒所需点数 *100',
  `unit`         VARCHAR(16) NOT NULL DEFAULT 'image',
  `default_params` JSON DEFAULT NULL,
  `group_code`   VARCHAR(64) NOT NULL                COMMENT '关联 account_group.code',
  `min_plan`     VARCHAR(32) NOT NULL DEFAULT 'free',
  `is_hot`       TINYINT NOT NULL DEFAULT 0,
  `is_new`       TINYINT NOT NULL DEFAULT 0,
  `sort`         INT NOT NULL DEFAULT 0,
  `status`       TINYINT NOT NULL DEFAULT 1,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`   DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_kind_status` (`kind`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='可用模型';

CREATE TABLE IF NOT EXISTS `plan` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `code`           VARCHAR(32) NOT NULL,
  `name`           VARCHAR(64) NOT NULL,
  `monthly_price`  BIGINT NOT NULL DEFAULT 0,
  `yearly_price`   BIGINT NOT NULL DEFAULT 0,
  `monthly_points` BIGINT NOT NULL DEFAULT 0,
  `rpm_limit`      INT NOT NULL DEFAULT 60,
  `concurrency`    INT NOT NULL DEFAULT 2,
  `model_scope`    JSON DEFAULT NULL,
  `feature`        JSON DEFAULT NULL,
  `status`         TINYINT NOT NULL DEFAULT 1,
  `sort`           INT NOT NULL DEFAULT 0,
  `created_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='套餐';

CREATE TABLE IF NOT EXISTS `user_subscription` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `plan_code`  VARCHAR(32) NOT NULL,
  `start_at`   DATETIME(3) NOT NULL,
  `expire_at`  DATETIME(3) NOT NULL,
  `auto_renew` TINYINT NOT NULL DEFAULT 0,
  `source`     VARCHAR(32) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_user_expire` (`user_id`, `expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户订阅';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `user_subscription`;
DROP TABLE IF EXISTS `plan`;
DROP TABLE IF EXISTS `model`;
