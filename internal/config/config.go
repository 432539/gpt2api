package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Log       LogConfig       `mapstructure:"log"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	Redis     RedisConfig     `mapstructure:"redis"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Crypto    CryptoConfig    `mapstructure:"crypto"`
	Security  SecurityConfig  `mapstructure:"security"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Upstream  UpstreamConfig  `mapstructure:"upstream"`
	MiniMax   MiniMaxConfig   `mapstructure:"minimax"`
	EPay      EPayConfig      `mapstructure:"epay"`
	Backup    BackupConfig    `mapstructure:"backup"`
	SMTP      SMTPConfig      `mapstructure:"smtp"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Env     string `mapstructure:"env"`
	Listen  string `mapstructure:"listen"`
	BaseURL string `mapstructure:"base_url"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type MySQLConfig struct {
	DSN                string `mapstructure:"dsn"`
	MaxOpenConns       int    `mapstructure:"max_open_conns"`
	MaxIdleConns       int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeSec int    `mapstructure:"conn_max_lifetime_sec"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type JWTConfig struct {
	Secret        string `mapstructure:"secret"`
	AccessTTLSec  int    `mapstructure:"access_ttl_sec"`
	RefreshTTLSec int    `mapstructure:"refresh_ttl_sec"`
	Issuer        string `mapstructure:"issuer"`
}

type CryptoConfig struct {
	AESKey string `mapstructure:"aes_key"`
}

type SecurityConfig struct {
	BcryptCost  int      `mapstructure:"bcrypt_cost"`
	CORSOrigins []string `mapstructure:"cors_origins"`
}

type SchedulerConfig struct {
	MinIntervalSec   int     `mapstructure:"min_interval_sec"`
	DailyUsageRatio  float64 `mapstructure:"daily_usage_ratio"`
	LockTTLSec       int     `mapstructure:"lock_ttl_sec"`
	Cooldown429Sec   int     `mapstructure:"cooldown_429_sec"`
	WarnedPauseHours int     `mapstructure:"warned_pause_hours"`
}

type UpstreamConfig struct {
	BaseURL            string `mapstructure:"base_url"`
	RequestTimeoutSec  int    `mapstructure:"request_timeout_sec"`
	SSEReadTimeoutSec  int    `mapstructure:"sse_read_timeout_sec"`
}

// MiniMaxConfig MiniMax 官方 API 配置。
// APIKey 为空时 MiniMax 模型路由会返回 503。
type MiniMaxConfig struct {
	// APIKey MiniMax 平台 API Key(Bearer Token)。
	// 环境变量:GPT2API_MINIMAX_API_KEY
	APIKey string `mapstructure:"api_key"`
	// BaseURL 覆盖默认 API 地址,留空使用 https://api.minimax.chat/v1
	// 可用于私有化部署或代理测试。
	BaseURL string `mapstructure:"base_url"`
}

// BackupConfig 数据库备份配置。
type BackupConfig struct {
	Dir           string `mapstructure:"dir"`            // 备份落盘目录,默认 /app/data/backups
	Retention     int    `mapstructure:"retention"`      // 保留最近 N 个(>0),0 表示不自动清理
	MysqldumpBin  string `mapstructure:"mysqldump_bin"`  // 默认 mysqldump
	MysqlBin      string `mapstructure:"mysql_bin"`      // 恢复用,默认 mysql
	MaxUploadMB   int    `mapstructure:"max_upload_mb"`  // 上传 .sql.gz 上限,默认 512
	AllowRestore  bool   `mapstructure:"allow_restore"`  // 是否允许 /restore 端点(生产强烈建议 false 手动切)
}

type EPayConfig struct {
	// GatewayURL 形如 https://pay.example.com/submit.php
	// 空字符串时整个充值通道被视为未启用,前端 list 会提示运维未配置。
	GatewayURL string `mapstructure:"gateway_url"`
	PID        string `mapstructure:"pid"`
	Key        string `mapstructure:"key"`
	// NotifyURL 后端异步回调(必填完整 https,不要带 query)
	NotifyURL string `mapstructure:"notify_url"`
	// ReturnURL 支付成功浏览器跳回(前端路由页,如 /billing)
	ReturnURL string `mapstructure:"return_url"`
	// SignType 目前只支持 MD5,保留扩展位。
	SignType string `mapstructure:"sign_type"`
	// Expires 订单默认有效期(分钟),0 取默认 30
	ExpiresMin int `mapstructure:"expires_min"`
}

// SMTPConfig 用于注册欢迎 / 充值到账 邮件通知。
// Host 为空时邮件通道整体关闭,不影响主流程。
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`      // 显示的 From 地址
	FromName string `mapstructure:"from_name"` // 显示名
	UseTLS   bool   `mapstructure:"use_tls"`   // true 隐式 TLS(465),false STARTTLS(587)
}

var (
	global *Config
	once   sync.Once
)

func Load(path string) (*Config, error) {
	var loadErr error
	once.Do(func() {
		v := viper.New()
		v.SetConfigFile(path)
		v.SetEnvPrefix("GPT2API")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
		if err := v.ReadInConfig(); err != nil {
			loadErr = fmt.Errorf("read config: %w", err)
			return
		}
		var c Config
		if err := v.Unmarshal(&c); err != nil {
			loadErr = fmt.Errorf("unmarshal config: %w", err)
			return
		}
		global = &c
	})
	return global, loadErr
}

// Get 返回全局配置,仅在 Load 之后调用。
func Get() *Config {
	if global == nil {
		panic("config not loaded; call config.Load first")
	}
	return global
}
