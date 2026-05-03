-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `promo_code` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `code`         VARCHAR(32) NOT NULL,
  `name`         VARCHAR(64) NOT NULL,
  `discount_type` TINYINT NOT NULL                COMMENT '1满减 2折扣 3赠点',
  `discount_val` BIGINT NOT NULL,
  `min_amount`   BIGINT NOT NULL DEFAULT 0,
  `apply_to`     VARCHAR(64) NOT NULL DEFAULT 'all',
  `total_qty`    INT NOT NULL DEFAULT 0,
  `used_qty`     INT NOT NULL DEFAULT 0,
  `per_user_limit` INT NOT NULL DEFAULT 1,
  `start_at`     DATETIME(3) NOT NULL,
  `end_at`       DATETIME(3) NOT NULL,
  `status`       TINYINT NOT NULL DEFAULT 1,
  `created_by`   BIGINT UNSIGNED DEFAULT NULL,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_status_time` (`status`, `start_at`, `end_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='优惠码';

CREATE TABLE IF NOT EXISTS `promo_code_use` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `promo_id`   BIGINT UNSIGNED NOT NULL,
  `code`       VARCHAR(32) NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `order_no`   VARCHAR(32) DEFAULT NULL,
  `discount`   BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_promo_user_order` (`promo_id`, `user_id`, `order_no`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='优惠码使用';

CREATE TABLE IF NOT EXISTS `redeem_code_batch` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `batch_no`     VARCHAR(32) NOT NULL,
  `name`         VARCHAR(64) NOT NULL,
  `reward_type`  VARCHAR(32) NOT NULL,
  `reward_value` JSON NOT NULL,
  `total_qty`    INT NOT NULL,
  `used_qty`     INT NOT NULL DEFAULT 0,
  `per_user_limit` INT NOT NULL DEFAULT 1,
  `expire_at`    DATETIME(3) DEFAULT NULL,
  `status`       TINYINT NOT NULL DEFAULT 1,
  `created_by`   BIGINT UNSIGNED DEFAULT NULL,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_batch_no` (`batch_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='CDK 批次';

CREATE TABLE IF NOT EXISTS `redeem_code` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `batch_id`     BIGINT UNSIGNED NOT NULL,
  `code`         VARCHAR(32) NOT NULL,
  `status`       TINYINT NOT NULL DEFAULT 0        COMMENT '0未使用 1已使用 2作废',
  `used_by`      BIGINT UNSIGNED DEFAULT NULL,
  `used_at`      DATETIME(3) DEFAULT NULL,
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_batch_status` (`batch_id`, `status`),
  KEY `idx_used_by` (`used_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='CDK';

CREATE TABLE IF NOT EXISTS `invitation_reward` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `inviter_id` BIGINT UNSIGNED NOT NULL,
  `invitee_id` BIGINT UNSIGNED NOT NULL,
  `kind`       VARCHAR(32) NOT NULL,
  `points`     BIGINT NOT NULL,
  `from_order` VARCHAR(32) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_inviter_created` (`inviter_id`, `created_at`),
  KEY `idx_invitee` (`invitee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邀请奖励';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `invitation_reward`;
DROP TABLE IF EXISTS `redeem_code`;
DROP TABLE IF EXISTS `redeem_code_batch`;
DROP TABLE IF EXISTS `promo_code_use`;
DROP TABLE IF EXISTS `promo_code`;
