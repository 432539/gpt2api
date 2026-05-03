CREATE TABLE IF NOT EXISTS `generation_upstream_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_id` char(26) NOT NULL,
  `provider` varchar(32) NOT NULL,
  `account_id` bigint unsigned DEFAULT NULL,
  `stage` varchar(64) NOT NULL,
  `method` varchar(12) DEFAULT NULL,
  `url` varchar(512) DEFAULT NULL,
  `status_code` int NOT NULL DEFAULT 0,
  `duration_ms` bigint NOT NULL DEFAULT 0,
  `request_excerpt` mediumtext,
  `response_excerpt` mediumtext,
  `error` text,
  `meta` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_task_id` (`task_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='provider upstream diagnostics';
