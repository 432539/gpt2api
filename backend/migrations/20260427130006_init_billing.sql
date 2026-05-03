-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `wallet_log` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`       BIGINT UNSIGNED NOT NULL,
  `direction`     TINYINT NOT NULL                  COMMENT '1 收入 -1 支出',
  `biz_type`      VARCHAR(32) NOT NULL,
  `biz_id`        VARCHAR(64) NOT NULL,
  `points`        BIGINT NOT NULL,
  `points_before` BIGINT NOT NULL,
  `points_after`  BIGINT NOT NULL,
  `remark`        VARCHAR(255) DEFAULT NULL,
  `created_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_user_created` (`user_id`, `created_at`),
  KEY `idx_biz` (`biz_type`, `biz_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='点数流水（总账）';

CREATE TABLE IF NOT EXISTS `recharge_record` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `order_no`      VARCHAR(32) NOT NULL,
  `user_id`       BIGINT UNSIGNED NOT NULL,
  `channel`       VARCHAR(32) NOT NULL,
  `amount`        BIGINT NOT NULL                   COMMENT '分',
  `points`        BIGINT NOT NULL,
  `bonus_points`  BIGINT NOT NULL DEFAULT 0,
  `status`        TINYINT NOT NULL DEFAULT 0,
  `paid_at`       DATETIME(3) DEFAULT NULL,
  `channel_trade_no` VARCHAR(64) DEFAULT NULL,
  `client_ip`     VARCHAR(45) DEFAULT NULL,
  `extra`         JSON DEFAULT NULL,
  `created_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_no` (`order_no`),
  KEY `idx_user_status` (`user_id`, `status`),
  KEY `idx_channel_trade` (`channel`, `channel_trade_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='充值记录';

CREATE TABLE IF NOT EXISTS `consume_record` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`      CHAR(26) NOT NULL,
  `user_id`      BIGINT UNSIGNED NOT NULL,
  `kind`         VARCHAR(16) NOT NULL,
  `model_code`   VARCHAR(64) NOT NULL,
  `count`        INT NOT NULL,
  `unit_points`  BIGINT NOT NULL,
  `total_points` BIGINT NOT NULL,
  `status`       TINYINT NOT NULL                  COMMENT '0预扣 1成功 2退款',
  `account_id`   BIGINT UNSIGNED DEFAULT NULL,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task` (`task_id`),
  KEY `idx_user_created` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='消费记录';

CREATE TABLE IF NOT EXISTS `refund_record` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`    CHAR(26) NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `points`     BIGINT NOT NULL,
  `reason`     VARCHAR(255) NOT NULL,
  `operator`   VARCHAR(64) NOT NULL DEFAULT 'system',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_task` (`task_id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='退款记录';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `refund_record`;
DROP TABLE IF EXISTS `consume_record`;
DROP TABLE IF EXISTS `recharge_record`;
DROP TABLE IF EXISTS `wallet_log`;
