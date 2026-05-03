-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `user` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uuid`            CHAR(36)        NOT NULL                  COMMENT '业务 ID',
  `email`           VARCHAR(128)    DEFAULT NULL,
  `phone`           VARCHAR(20)     DEFAULT NULL,
  `username`        VARCHAR(64)     DEFAULT NULL              COMMENT '昵称',
  `avatar`          VARCHAR(255)    DEFAULT NULL,
  `password`        VARCHAR(72)     NOT NULL                  COMMENT 'bcrypt',
  `points`          BIGINT          NOT NULL DEFAULT 0        COMMENT '可用点数 *100',
  `frozen_points`   BIGINT          NOT NULL DEFAULT 0        COMMENT '冻结点数 *100',
  `total_recharge`  BIGINT          NOT NULL DEFAULT 0        COMMENT '累计充值（分）',
  `plan_code`       VARCHAR(32)     NOT NULL DEFAULT 'free',
  `plan_expire_at`  DATETIME(3)     DEFAULT NULL,
  `inviter_id`      BIGINT UNSIGNED DEFAULT NULL,
  `invite_code`     VARCHAR(16)     NOT NULL,
  `status`          TINYINT         NOT NULL DEFAULT 1        COMMENT '1启用 0禁用 -1注销',
  `register_ip`     VARCHAR(45)     DEFAULT NULL,
  `last_login_at`   DATETIME(3)     DEFAULT NULL,
  `last_login_ip`   VARCHAR(45)     DEFAULT NULL,
  `created_at`      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`      DATETIME(3)     DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_uuid` (`uuid`),
  UNIQUE KEY `uk_email` (`email`),
  UNIQUE KEY `uk_phone` (`phone`),
  UNIQUE KEY `uk_invite_code` (`invite_code`),
  KEY `idx_inviter` (`inviter_id`),
  KEY `idx_status_created` (`status`, `created_at`),
  KEY `idx_deleted` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户';

CREATE TABLE IF NOT EXISTS `user_profile` (
  `user_id`     BIGINT UNSIGNED NOT NULL,
  `gender`      TINYINT     DEFAULT 0,
  `birthday`    DATE        DEFAULT NULL,
  `bio`         VARCHAR(255) DEFAULT NULL,
  `prefer_lang` VARCHAR(10) DEFAULT 'zh-CN',
  `setting`     JSON        DEFAULT NULL,
  `updated_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户扩展资料';

CREATE TABLE IF NOT EXISTS `user_invite_relation` (
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `inviter_id` BIGINT UNSIGNED NOT NULL,
  `invite_code` VARCHAR(16) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`user_id`),
  KEY `idx_inviter` (`inviter_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邀请关系';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `user_invite_relation`;
DROP TABLE IF EXISTS `user_profile`;
DROP TABLE IF EXISTS `user`;
