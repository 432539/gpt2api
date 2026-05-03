// Package config 加载 KleinAI 全局配置。
// 优先级：环境变量 > config.${KLEIN_ENV}.yaml > config.yaml。
package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App       App       `mapstructure:"app"`
	Server    Server    `mapstructure:"server"`
	MySQL     MySQL     `mapstructure:"mysql"`
	Redis     Redis     `mapstructure:"redis"`
	JWT       JWT       `mapstructure:"jwt"`
	Logger    Logger    `mapstructure:"logger"`
	Snowflake Snowflake `mapstructure:"snowflake"`
	CORS      CORS      `mapstructure:"cors"`
	RateLimit RateLimit `mapstructure:"ratelimit"`
	Pool      Pool      `mapstructure:"pool"`
	Provider  Provider  `mapstructure:"provider"`
	Billing   Billing   `mapstructure:"billing"`
	CDN       CDN       `mapstructure:"cdn"`
	AESKey    string    `mapstructure:"-"` // 来自环境变量
}

type App struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type Server struct {
	APIPort         int           `mapstructure:"api_port"`
	AdminPort       int           `mapstructure:"admin_port"`
	OpenAIPort      int           `mapstructure:"openai_port"`
	WSPort          int           `mapstructure:"ws_port"`
	PprofPort       int           `mapstructure:"pprof_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type MySQL struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
}

type Redis struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type JWT struct {
	Secret        string        `mapstructure:"-"`
	RefreshSecret string        `mapstructure:"-"`
	AccessTTL     time.Duration `mapstructure:"access_ttl"`
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`
}

type Logger struct {
	Level      string `mapstructure:"level"`
	Dir        string `mapstructure:"dir"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
	Console    bool   `mapstructure:"console"`
}

type Snowflake struct {
	NodeID int64 `mapstructure:"node_id"`
}

type CORS struct {
	Origins []string `mapstructure:"origins"`
}

type RateLimit struct {
	IPPerMinute     int `mapstructure:"ip_per_minute"`
	UserPerMinute   int `mapstructure:"user_per_minute"`
	APIKeyPerMinute int `mapstructure:"apikey_per_minute"`
}

type Pool struct {
	Strategy           string `mapstructure:"strategy"`
	CooldownSeconds    int    `mapstructure:"cooldown_seconds"`
	FailThreshold      int    `mapstructure:"fail_threshold"`
	HealthCheckSeconds int    `mapstructure:"health_check_seconds"`
}

type Provider struct {
	OpenAIBase     string        `mapstructure:"openai_base"`
	GrokBase       string        `mapstructure:"grok_base"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	Retry          int           `mapstructure:"retry"`
}

type Billing struct {
	PointUnit int64 `mapstructure:"point_unit"`
}

type CDN struct {
	Base string `mapstructure:"base"`
}

var (
	cfg     *Config
	once    sync.Once
	loadErr error
)

// Load 读取配置（线程安全，仅生效一次）。
func Load() (*Config, error) {
	once.Do(func() {
		cfg, loadErr = loadInternal()
	})
	return cfg, loadErr
}

// MustLoad 失败直接 panic（仅 cmd/* 入口使用）。
func MustLoad() *Config {
	c, err := Load()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}
	return c
}

// Get 返回已加载的配置；若未 Load 会 panic。
func Get() *Config {
	if cfg == nil {
		panic("config not loaded")
	}
	return cfg
}

func loadInternal() (*Config, error) {
	env := strings.TrimSpace(os.Getenv("KLEIN_ENV"))
	if env == "" {
		env = "dev"
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs")
	v.AddConfigPath("../configs")
	v.AddConfigPath("../../configs")

	v.SetConfigName("config")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read base config: %w", err)
	}

	v.SetConfigName("config." + env)
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !asErr(err, &notFound) {
			return nil, fmt.Errorf("merge env config: %w", err)
		}
	}

	v.SetEnvPrefix("KLEIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	out := &Config{}
	if err := v.Unmarshal(out); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	mapEnv := func(target *string, key string) {
		if val := os.Getenv(key); val != "" {
			*target = val
		}
	}
	mapEnv(&out.MySQL.DSN, "KLEIN_DB_DSN")
	mapEnv(&out.Redis.Addr, "KLEIN_REDIS_ADDR")
	mapEnv(&out.Redis.Password, "KLEIN_REDIS_PASSWORD")
	mapEnv(&out.JWT.Secret, "KLEIN_JWT_SECRET")
	mapEnv(&out.JWT.RefreshSecret, "KLEIN_JWT_REFRESH_SECRET")
	mapEnv(&out.AESKey, "KLEIN_AES_KEY")
	mapEnv(&out.Provider.OpenAIBase, "KLEIN_OPENAI_BASE")
	mapEnv(&out.Provider.GrokBase, "KLEIN_GROK_BASE")
	mapEnv(&out.Logger.Dir, "KLEIN_LOG_DIR")
	mapEnv(&out.Logger.Level, "KLEIN_LOG_LEVEL")

	if origins := os.Getenv("KLEIN_CORS_ORIGINS"); origins != "" {
		out.CORS.Origins = splitAndTrim(origins, ",")
	}

	if env == "prod" {
		if err := validateProd(out); err != nil {
			return nil, err
		}
	}

	out.App.Env = env
	return out, nil
}

func validateProd(c *Config) error {
	if c.MySQL.DSN == "" {
		return fmt.Errorf("KLEIN_DB_DSN is required in prod")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("KLEIN_REDIS_ADDR is required in prod")
	}
	if len(c.JWT.Secret) < 32 || len(c.JWT.RefreshSecret) < 32 {
		return fmt.Errorf("KLEIN_JWT_SECRET / KLEIN_JWT_REFRESH_SECRET must be >= 32 bytes")
	}
	if len(c.AESKey) < 32 {
		return fmt.Errorf("KLEIN_AES_KEY must be >= 32 bytes")
	}
	return nil
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// asErr 是 errors.As 的薄包装，避免 import cycle。
func asErr(err error, target any) bool {
	type asInterface interface{ As(any) bool }
	if a, ok := err.(asInterface); ok && a.As(target) {
		return true
	}
	return false
}

// IsProd 判断是否生产环境。
func (c *Config) IsProd() bool { return c.App.Env == "prod" }

// IsDev 判断是否开发环境。
func (c *Config) IsDev() bool { return c.App.Env == "dev" || c.App.Env == "local" }
