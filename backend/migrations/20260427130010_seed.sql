-- +goose Up
-- +goose StatementBegin

-- 1. 后台角色
INSERT INTO `admin_role` (`name`, `code`, `remark`) VALUES
  ('超级管理员', 'super', '拥有所有权限'),
  ('运营',      'ops',   '日常运营，无法管理超管'),
  ('客服',      'cs',    '只读 + 用户充值/退款'),
  ('风控',      'risk',  '封禁/解封/审计')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 1.1 默认超级管理员账号
-- 用户名: admin  密码: admin123 （bcrypt cost=12，请上线后立即在管理后台修改）
INSERT INTO `admin_user` (`username`, `password`, `nickname`, `role_id`, `status`)
SELECT 'admin',
       '$2a$12$4a8W/7ZL9nnFMnlwdXn2uOhkDYX53cnOUEovWnXs7XoA./alaTmeS',
       '系统管理员',
       (SELECT `id` FROM `admin_role` WHERE `code`='super' LIMIT 1),
       1
WHERE NOT EXISTS (SELECT 1 FROM `admin_user` WHERE `username`='admin');

-- 2. 套餐
INSERT INTO `plan` (`code`, `name`, `monthly_price`, `yearly_price`, `monthly_points`, `rpm_limit`, `concurrency`, `sort`)
VALUES
  ('free', '免费版', 0,    0,     10000,  30,  1, 1),
  ('pro',  'Pro',    2900, 29900, 100000, 120, 4, 2),
  ('max',  'Max',    9900, 99900, 500000, 300, 8, 3)
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 3. 默认账号池分组
INSERT INTO `account_group` (`provider`, `code`, `name`, `strategy`)
VALUES
  ('gpt',  'gpt-image-default',   'GPT 通用生图', 'round_robin'),
  ('grok', 'grok-video-default',  'GROK 通用生视频', 'round_robin')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 4. 模型
INSERT INTO `model` (`code`, `name`, `kind`, `provider`, `version`, `tags`, `point_per_unit`, `unit`, `group_code`, `min_plan`, `is_hot`, `sort`)
VALUES
  ('img-v3',     '通用模型 V3.0', 'image', 'gpt',  'v3.0', '通用,写实,海报', 400, 'image',  'gpt-image-default',  'free', 1, 1),
  ('img-real',   '写实 V2.1',     'image', 'gpt',  'v2.1', '写实,人像,摄影', 400, 'image',  'gpt-image-default',  'free', 0, 2),
  ('img-anime',  '二次元 V2.0',   'image', 'gpt',  'v2.0', '二次元,漫画',     300, 'image',  'gpt-image-default',  'free', 1, 3),
  ('img-3d',     '3D 渲染 V2.0',  'image', 'gpt',  'v2.0', '3D,概念,渲染',    500, 'image',  'gpt-image-default',  'pro',  0, 4),
  ('vid-v1',     '文生视频 V1.0', 'video', 'grok', 'v1.0', '通用',            1500, 'second', 'grok-video-default', 'free', 1, 5),
  ('vid-i2v',    '图生视频 V1.0', 'video', 'grok', 'v1.0', '动画',            2000, 'second', 'grok-video-default', 'pro',  0, 6)
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

-- 5. 系统配置
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
  ('points.cny_rate',     '100',                              '1 元 = N 点（最小单位 0.01）'),
  ('pool.strategy',       '"round_robin"',                    '调度策略'),
  ('pool.fail_threshold', '5',                                '熔断失败次数'),
  ('pool.cooldown_sec',   '600',                              '熔断冷却秒'),
  ('invite.first_recharge_reward', '5000',                    '首充返点（点 *100）'),
  ('invite.lifetime_share_pct',    '5',                       '终身分润 %')
ON DUPLICATE KEY UPDATE `value`=VALUES(`value`);

-- 6. 字典
INSERT INTO `system_dict` (`dict_group`, `dict_key`, `dict_value`, `sort`) VALUES
  ('image_ratio', '1:1',   '正方形',  1),
  ('image_ratio', '3:4',   '竖版',    2),
  ('image_ratio', '4:3',   '横版',    3),
  ('image_ratio', '16:9',  '宽屏',    4),
  ('image_ratio', '9:16',  '手机壁纸', 5),
  ('video_dur',   '4',     '4 秒',    1),
  ('video_dur',   '8',     '8 秒',    2),
  ('video_dur',   '16',    '16 秒',   3)
ON DUPLICATE KEY UPDATE `dict_value`=VALUES(`dict_value`);

-- +goose StatementEnd

-- +goose Down
DELETE FROM `system_dict`;
DELETE FROM `system_config`;
DELETE FROM `model`;
DELETE FROM `account_group`;
DELETE FROM `plan`;
DELETE FROM `admin_role`;
