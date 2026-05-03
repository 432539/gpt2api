-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `generation_task` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`        CHAR(26) NOT NULL,
  `user_id`        BIGINT UNSIGNED NOT NULL,
  `kind`           VARCHAR(16) NOT NULL              COMMENT 'image / video',
  `mode`           VARCHAR(16) NOT NULL              COMMENT 't2i / i2i / t2v / i2v',
  `model_code`     VARCHAR(64) NOT NULL,
  `prompt`         TEXT NOT NULL,
  `neg_prompt`     TEXT DEFAULT NULL,
  `params`         JSON NOT NULL,
  `ref_assets`     JSON DEFAULT NULL,
  `count`          INT NOT NULL DEFAULT 1,
  `cost_points`    BIGINT NOT NULL,
  `idem_key`       VARCHAR(64) NOT NULL,
  `account_id`     BIGINT UNSIGNED DEFAULT NULL,
  `provider`       VARCHAR(32) NOT NULL,
  `status`         TINYINT NOT NULL DEFAULT 0        COMMENT '0待处理 1进行中 2成功 3失败 4已退点',
  `progress`       TINYINT NOT NULL DEFAULT 0,
  `error`          VARCHAR(255) DEFAULT NULL,
  `started_at`     DATETIME(3) DEFAULT NULL,
  `finished_at`    DATETIME(3) DEFAULT NULL,
  `client_ip`      VARCHAR(45) DEFAULT NULL,
  `from_api_key_id` BIGINT UNSIGNED DEFAULT NULL,
  `created_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`     DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_id` (`task_id`),
  UNIQUE KEY `uk_user_idem` (`user_id`, `idem_key`),
  KEY `idx_user_kind_status` (`user_id`, `kind`, `status`),
  KEY `idx_status_created` (`status`, `created_at`),
  KEY `idx_account` (`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='生成任务';

CREATE TABLE IF NOT EXISTS `generation_result` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`    CHAR(26) NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `kind`       VARCHAR(16) NOT NULL,
  `seq`        TINYINT NOT NULL DEFAULT 0,
  `url`        VARCHAR(512) NOT NULL,
  `thumb_url`  VARCHAR(512) DEFAULT NULL,
  `width`      INT DEFAULT NULL,
  `height`     INT DEFAULT NULL,
  `duration_ms` INT DEFAULT NULL,
  `size_bytes` BIGINT DEFAULT NULL,
  `meta`       JSON DEFAULT NULL,
  `is_public`  TINYINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task` (`task_id`),
  KEY `idx_user_kind` (`user_id`, `kind`),
  KEY `idx_public_created` (`is_public`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='生成结果';

CREATE TABLE IF NOT EXISTS `prompt_history` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `kind`       VARCHAR(16) NOT NULL,
  `prompt`     TEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_user_created` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='提示词历史';

CREATE TABLE IF NOT EXISTS `favorite` (
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `result_id`  BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_id`, `result_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='收藏';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `favorite`;
DROP TABLE IF EXISTS `prompt_history`;
DROP TABLE IF EXISTS `generation_result`;
DROP TABLE IF EXISTS `generation_task`;
