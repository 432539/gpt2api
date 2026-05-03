-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `admin_role` (
  `id`        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name`      VARCHAR(64) NOT NULL,
  `code`      VARCHAR(32) NOT NULL,
  `remark`    VARCHAR(255) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='后台角色';

CREATE TABLE IF NOT EXISTS `admin_role_permission` (
  `role_id`     BIGINT UNSIGNED NOT NULL,
  `permission`  VARCHAR(128) NOT NULL,
  PRIMARY KEY (`role_id`, `permission`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='角色权限';

CREATE TABLE IF NOT EXISTS `admin_user` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username`   VARCHAR(64) NOT NULL,
  `password`   VARCHAR(72) NOT NULL,
  `nickname`   VARCHAR(64) DEFAULT NULL,
  `email`      VARCHAR(128) DEFAULT NULL,
  `role_id`    BIGINT UNSIGNED NOT NULL,
  `status`     TINYINT NOT NULL DEFAULT 1,
  `last_login_at` DATETIME(3) DEFAULT NULL,
  `last_login_ip` VARCHAR(45) DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='后台账号';

CREATE TABLE IF NOT EXISTS `admin_audit_log` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `admin_id`      BIGINT UNSIGNED NOT NULL,
  `admin_name`    VARCHAR(64) NOT NULL,
  `module`        VARCHAR(64) NOT NULL,
  `action`        VARCHAR(64) NOT NULL,
  `target_type`   VARCHAR(64) DEFAULT NULL,
  `target_id`     VARCHAR(64) DEFAULT NULL,
  `before_value`  JSON DEFAULT NULL,
  `after_value`   JSON DEFAULT NULL,
  `ip`            VARCHAR(45) DEFAULT NULL,
  `ua`            VARCHAR(255) DEFAULT NULL,
  `created_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_admin` (`admin_id`),
  KEY `idx_module_action` (`module`, `action`),
  KEY `idx_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='后台操作审计';
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS `admin_audit_log`;
DROP TABLE IF EXISTS `admin_user`;
DROP TABLE IF EXISTS `admin_role_permission`;
DROP TABLE IF EXISTS `admin_role`;
