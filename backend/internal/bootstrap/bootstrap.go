// Package bootstrap 集中初始化所有基础设施，供 cmd/* 复用。
package bootstrap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kleinai/backend/pkg/config"
	"github.com/kleinai/backend/pkg/crypto"
	"github.com/kleinai/backend/pkg/database"
	"github.com/kleinai/backend/pkg/jwtx"
	"github.com/kleinai/backend/pkg/logger"
	"github.com/kleinai/backend/pkg/ratelimit"
	"github.com/kleinai/backend/pkg/snowflake"
	"github.com/kleinai/backend/pkg/version"
)

// Deps 启动后向业务层注入的依赖集合。
type Deps struct {
	Cfg     *config.Config
	DB      *gorm.DB
	Redis   *redis.Client
	JWT     *jwtx.Manager
	Limiter *ratelimit.Limiter
	AES     *crypto.AESGCM
}

// Init 完整初始化（config / logger / mysql / redis / jwt / aes / snowflake）。
func Init(serviceName string) (*Deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := logger.Init(cfg); err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}
	logger.L().Info("kleinai starting",
		zap.String("service", serviceName),
		zap.String("env", cfg.App.Env),
		zap.String("version", version.Info()),
	)

	if err := snowflake.Init(cfg.Snowflake.NodeID); err != nil {
		return nil, err
	}

	jwtMgr, err := jwtx.New(cfg.JWT.Secret, cfg.JWT.RefreshSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("init jwt: %w", err)
	}

	aes, err := initAES(cfg.AESKey)
	if err != nil {
		return nil, fmt.Errorf("init aes: %w", err)
	}

	db, err := database.NewMySQL(&cfg.MySQL)
	if err != nil {
		// dev 下允许暂时跑空依赖（仅 healthz 可用），但日志告警
		if cfg.IsDev() {
			logger.L().Warn("mysql unavailable, running in degraded mode", zap.Error(err))
			db = nil
		} else {
			return nil, err
		}
	}

	rdb, err := database.NewRedis(&cfg.Redis)
	if err != nil {
		if cfg.IsDev() {
			logger.L().Warn("redis unavailable, running in degraded mode", zap.Error(err))
			rdb = nil
		} else {
			return nil, err
		}
	}

	var limiter *ratelimit.Limiter
	if rdb != nil {
		limiter = ratelimit.New(rdb)
	}

	return &Deps{
		Cfg:     cfg,
		DB:      db,
		Redis:   rdb,
		JWT:     jwtMgr,
		Limiter: limiter,
		AES:     aes,
	}, nil
}

func initAES(raw string) (*crypto.AESGCM, error) {
	if raw == "" {
		return nil, nil
	}
	key, err := decodeAESKey(raw)
	if err != nil {
		return nil, err
	}
	return crypto.NewAESGCM(key)
}

func decodeAESKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, errors.New("KLEIN_AES_KEY must be 32 bytes raw or 64 hex chars")
}

// Run 优雅启停 HTTP 服务。
func Run(srv *http.Server, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		logger.L().Info("http listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.L().Info("shutting down http server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Sync()
	logger.L().Info("graceful shutdown done")
	return nil
}
