// Package logger 封装 zap + lumberjack。
// 严禁业务代码使用 fmt.Println / log.Println。
package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/kleinai/backend/pkg/config"
)

type ctxKey struct{}

var (
	base    *zap.Logger
	once    sync.Once
	initErr error
)

// Init 初始化全局 logger。
func Init(c *config.Config) error {
	once.Do(func() {
		initErr = doInit(c)
	})
	return initErr
}

func doInit(c *config.Config) error {
	level := zap.InfoLevel
	if err := level.UnmarshalText([]byte(c.Logger.Level)); err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}

	cores := make([]zapcore.Core, 0, 2)

	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	if c.Logger.Console || c.IsDev() {
		cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), level))
	}

	if c.Logger.Dir != "" {
		if err := os.MkdirAll(c.Logger.Dir, 0o755); err != nil {
			return fmt.Errorf("mkdir log dir: %w", err)
		}
		writer := &lumberjack.Logger{
			Filename:   filepath.Join(c.Logger.Dir, "app.log"),
			MaxSize:    c.Logger.MaxSizeMB,
			MaxAge:     c.Logger.MaxAgeDays,
			LocalTime:  true,
			Compress:   c.Logger.Compress,
			MaxBackups: 30,
		}
		cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(writer), level))
	}

	base = zap.New(zapcore.NewTee(cores...),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
		zap.Fields(zap.String("app", c.App.Name), zap.String("env", c.App.Env)),
	)
	zap.ReplaceGlobals(base)
	return nil
}

// L 返回全局 logger（未初始化时返回 NoOp）。
func L() *zap.Logger {
	if base == nil {
		return zap.NewNop()
	}
	return base
}

// FromCtx 从 context 中取出携带 trace_id 等字段的 logger。
func FromCtx(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return L()
}

// Inject 把带额外字段的 logger 放入 context。
func Inject(ctx context.Context, fields ...zap.Field) context.Context {
	l := FromCtx(ctx).With(fields...)
	return context.WithValue(ctx, ctxKey{}, l)
}

// Sync 在程序退出前刷盘。
func Sync() {
	if base != nil {
		_ = base.Sync()
	}
}
