-- +goose Up
-- +goose StatementBegin
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
  ('grok.cf.enabled', 'true', 'GROK Cloudflare cookie 自动刷新开关'),
  ('grok.cf.flaresolverr_url', '"http://flaresolverr:8191"', '内置 FlareSolverr 地址'),
  ('grok.cf.refresh_interval_seconds', '600', 'GROK CF cookie 刷新间隔'),
  ('grok.cf.timeout_seconds', '90', 'FlareSolverr 单次解题超时'),
  ('grok.cf.cookies', '""', '最近一次 FlareSolverr 获取的 Cookie'),
  ('grok.cf.clearance', '""', '最近一次 FlareSolverr 获取的 cf_clearance'),
  ('grok.cf.user_agent', '""', 'FlareSolverr 浏览器 User-Agent'),
  ('grok.cf.browser', '""', 'FlareSolverr 浏览器类型'),
  ('grok.cf.last_error', '""', '最近一次 FlareSolverr 刷新错误'),
  ('grok.cf.last_refresh_at', '0', '最近一次 FlareSolverr 成功刷新时间')
ON DUPLICATE KEY UPDATE `remark`=VALUES(`remark`);
-- +goose StatementEnd

-- +goose Down
DELETE FROM `system_config`
WHERE `key` IN (
  'grok.cf.enabled',
  'grok.cf.flaresolverr_url',
  'grok.cf.refresh_interval_seconds',
  'grok.cf.timeout_seconds',
  'grok.cf.cookies',
  'grok.cf.clearance',
  'grok.cf.user_agent',
  'grok.cf.browser',
  'grok.cf.last_error',
  'grok.cf.last_refresh_at'
);
