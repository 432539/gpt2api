package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/account"
	"github.com/432539/gpt2api/internal/apikey"
	"github.com/432539/gpt2api/internal/audit"
	"github.com/432539/gpt2api/internal/auth"
	"github.com/432539/gpt2api/internal/backup"
	"github.com/432539/gpt2api/internal/billing"
	"github.com/432539/gpt2api/internal/config"
	"github.com/432539/gpt2api/internal/db"
	"github.com/432539/gpt2api/internal/gateway"
	"github.com/432539/gpt2api/internal/image"
	modelpkg "github.com/432539/gpt2api/internal/model"
	"github.com/432539/gpt2api/internal/proxy"
	gwratelimit "github.com/432539/gpt2api/internal/ratelimit"
	"github.com/432539/gpt2api/internal/recharge"
	"github.com/432539/gpt2api/internal/scheduler"
	"github.com/432539/gpt2api/internal/server"
	"github.com/432539/gpt2api/internal/settings"
	"github.com/432539/gpt2api/internal/usage"
	"github.com/432539/gpt2api/internal/user"
	"github.com/432539/gpt2api/pkg/crypto"
	pkgjwt "github.com/432539/gpt2api/pkg/jwt"
	"github.com/432539/gpt2api/pkg/lock"
	"github.com/432539/gpt2api/pkg/logger"
	"github.com/432539/gpt2api/pkg/mailer"
	pkgratelimit "github.com/432539/gpt2api/pkg/ratelimit"
)

var (
	configPath = flag.String("c", "configs/config.yaml", "config file path")
	showVer    = flag.Bool("v", false, "show version and exit")
)

var (
	version   = "0.2.0-dev"
	buildTime = "unknown"
)

func main() {
	flag.Parse()
	if *showVer {
		fmt.Printf("gpt2api %s (build %s)\n", version, buildTime)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	log := logger.L()
	log.Info("boot gpt2api",
		zap.String("version", version),
		zap.String("env", cfg.App.Env),
		zap.String("listen", cfg.App.Listen),
	)

	sqldb, err := db.NewMySQL(cfg.MySQL)
	if err != nil {
		log.Fatal("mysql init", zap.Error(err))
	}
	defer sqldb.Close()

	rdb, err := db.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatal("redis init", zap.Error(err))
	}
	defer rdb.Close()

	cipher, err := crypto.NewAESGCM(cfg.Crypto.AESKey)
	if err != nil {
		log.Fatal("crypto init", zap.Error(err))
	}

	// ---- DAO / Service ----
	userDAO := user.NewDAO(sqldb)
	proxyDAO := proxy.NewDAO(sqldb)
	proxySvc := proxy.NewService(proxyDAO, cipher)

	accDAO := account.NewDAO(sqldb)
	accSvc := account.NewService(accDAO, cipher)

	keyDAO := apikey.NewDAO(sqldb)
	keySvc := apikey.NewService(keyDAO)

	modelDAO := modelpkg.NewDAO(sqldb)
	modelReg := modelpkg.NewRegistry(modelDAO)
	if err := modelReg.Preload(context.Background()); err != nil {
		log.Warn("model preload failed", zap.Error(err))
	}

	rl := lock.NewRedisLock(rdb)
	sched := scheduler.New(accSvc, proxySvc, rl, cfg.Scheduler)

	tb := pkgratelimit.NewTokenBucket(rdb)
	limiter := gwratelimit.New(tb)

	groupCache := user.NewGroupCache(userDAO, 30*time.Second)

	usageLogger := usage.New(sqldb, usage.Options{})
	defer usageLogger.Close()

	billEngine := billing.New(sqldb)

	jm := pkgjwt.NewManager(pkgjwt.Config{
		Secret:        cfg.JWT.Secret,
		Issuer:        cfg.JWT.Issuer,
		AccessTTLSec:  cfg.JWT.AccessTTLSec,
		RefreshTTLSec: cfg.JWT.RefreshTTLSec,
	})
	authSvc := auth.NewService(userDAO, jm, cfg.Security.BcryptCost)

	gwH := &gateway.Handler{
		Models:    modelReg,
		Keys:      keySvc,
		Billing:   billEngine,
		Scheduler: sched,
		Groups:    groupCache,
		Limiter:   limiter,
		Usage:     usageLogger,
		AccSvc:    accSvc,
	}

	imageDAO := image.NewDAO(sqldb)
	imageRunner := image.NewRunner(sched, imageDAO)
	imageStore := image.NewLocalStore(cfg.ImageStore.Dir)
	imageRunner.SetQuotaDecrementor(accDAO) // 生图成功后立即扣减账号额度
	imageRunner.SetLocalStore(imageStore)
	imagesH := &gateway.ImagesHandler{
		Handler:    gwH,
		Runner:     imageRunner,
		DAO:        imageDAO,
		LocalStore: imageStore,
	}
	gwH.Images = imagesH // chat/completions 识别到图像模型时转派

	auditDAO := audit.NewDAO(sqldb)
	auditH := audit.NewHandler(auditDAO)

	var backupH *backup.Handler
	backupDAO := backup.NewDAO(sqldb)
	if backupSvc, err := backup.New(cfg.Backup, cfg.MySQL, backupDAO); err != nil {
		// 备份功能是后台可选项,启动不应致命。降级为禁用,记录警告。
		log.Warn("backup service disabled", zap.Error(err))
	} else {
		backupH = backup.NewHandler(backupSvc, backupDAO, authSvc, auditDAO)
		log.Info("backup service ready", zap.String("dir", backupSvc.Dir()))
	}

	adminUserH := user.NewAdminHandler(userDAO, authSvc, billEngine, auditDAO)
	adminGroupH := user.NewAdminGroupHandler(userDAO, auditDAO)

	adminModelH := modelpkg.NewAdminHandler(modelDAO, modelReg, auditDAO)
	adminKeyH := apikey.NewAdminHandler(keySvc, keyDAO, sqldb)
	usageQDAO := usage.NewQueryDAO(sqldb)
	adminUsageH := usage.NewAdminHandler(usageQDAO)
	meUsageH := usage.NewMeHandler(usageQDAO)
	meImageH := image.NewMeHandler(imageDAO)
	adminImageH := image.NewAdminHandler(imageDAO)

	mailSvc := mailer.New(mailer.Config{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
		FromName: cfg.SMTP.FromName,
		UseTLS:   cfg.SMTP.UseTLS,
	}, log)
	defer mailSvc.Close()
	if mailSvc.Disabled() {
		log.Info("mail channel disabled (smtp.host empty)")
	} else {
		log.Info("mail channel ready", zap.String("host", cfg.SMTP.Host))
	}
	// 把 mailSvc 注入给 authSvc 用于注册欢迎邮件
	authSvc.SetMailer(mailSvc, cfg.App.BaseURL)

	// 系统设置(KV),启动时从 DB 装载到内存缓存;注入给 auth 用于注册开关/赠送积分。
	settingsDAO := settings.NewDAO(sqldb)
	settingsSvc := settings.NewService(settingsDAO)
	if err := settingsSvc.Reload(context.Background()); err != nil {
		log.Warn("settings reload failed, using defaults", zap.Error(err))
	}
	settingsH := settings.NewHandler(settingsSvc, mailSvc, auditDAO)
	authSvc.SetSettings(settingsSvc)
	authSvc.SetBilling(billEngine)

	// 把 settings 注入到其它受控业务(可热更)
	keySvc.SetSettings(settingsSvc)
	gwH.Settings = settingsSvc
	sched.SetRuntime(scheduler.RuntimeParams{
		DailyUsageRatio: settingsSvc.DailyUsageRatio,
		Cooldown429Sec:  settingsSvc.Cooldown429Sec,
		WarnedPauseHrs:  settingsSvc.WarnedPauseHours,
		QueueWaitSec:    settingsSvc.DispatchQueueWaitSec,
	})
	// JWT TTL 热更:每次 Issue 时从 settings 读取,<=0 回退启动值
	jm.SetTTLProvider(func() (int, int) {
		return settingsSvc.JWTAccessTTLSec(), settingsSvc.JWTRefreshTTLSec()
	})

	rechargeDAO := recharge.NewDAO(sqldb)
	rechargeSvc := recharge.NewService(rechargeDAO, billEngine, userDAO, cfg.EPay, mailSvc, cfg.App.BaseURL, log)
	rechargeSvc.SetSettings(settingsSvc)
	rechargeH := recharge.NewHandler(rechargeSvc)
	adminRechargeH := recharge.NewAdminHandler(rechargeSvc, authSvc)

	// 代理池健康探测器:由 settings 提供热更参数,注入到 Handler
	proxyH := proxy.NewHandler(proxySvc)
	prober := proxy.NewProber(proxySvc, settingsSvc, log.Named("proxy-prober"))
	proxyH.SetProber(prober)

	proberCtx, cancelProber := context.WithCancel(context.Background())
	defer cancelProber()
	go prober.Run(proberCtx)

	// 账号池:AT 刷新器 + 额度探测器
	accountH := account.NewHandler(accSvc)
	accRefresher := account.NewRefresher(accSvc, settingsSvc, log.Named("account-refresh"))
	accQuota := account.NewQuotaProber(accSvc, settingsSvc, log.Named("account-quota"))

	// 账号→代理 解析器:让 refresh / probe 走账号自身绑定的代理,
	// 避免从境内直连 chatgpt.com / auth.openai.com 时被中间设备 TLS 劫持。
	acctProxyResolver := &accountProxyResolver{accSvc: accSvc, proxySvc: proxySvc}
	accRefresher.SetProxyResolver(acctProxyResolver)
	accQuota.SetProxyResolver(acctProxyResolver)

	accountH.SetRefresher(accRefresher)
	accountH.SetProber(accQuota)
	accountH.SetSettings(settingsSvc)
	accountH.SetProxyResolver(acctProxyResolver)

	// 把 resolver 注入到图片代理端点:下载图片时按 account_id 解出 AT/cookies/proxy。
	imagesH.ImageAccResolver = acctProxyResolver

	accBgCtx, cancelAccBg := context.WithCancel(context.Background())
	defer cancelAccBg()
	go accRefresher.Run(accBgCtx)
	go accQuota.Run(accBgCtx)

	deps := &server.Deps{
		Config: cfg,
		JWT:    jm,

		AuthH: auth.NewHandler(authSvc),
		UserH: user.NewHandler(userDAO),

		KeySvc:   keySvc,
		KeyH:     apikey.NewHandler(keySvc),
		ProxyH:   proxyH,
		AccountH: accountH,

		GatewayH: gwH,
		ImagesH:  imagesH,

		BackupH:     backupH,
		AuditH:      auditH,
		AuditDAO:    auditDAO,
		AdminUserH:  adminUserH,
		AdminGroupH: adminGroupH,

		AdminModelH: adminModelH,
		AdminKeyH:   adminKeyH,
		AdminUsageH: adminUsageH,

		MeUsageH:    meUsageH,
		MeImageH:    meImageH,
		AdminImageH: adminImageH,

		RechargeH:      rechargeH,
		AdminRechargeH: adminRechargeH,

		SettingsH: settingsH,
	}

	r := server.New(deps)
	srv := &http.Server{
		Addr:              cfg.App.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("http server started", zap.String("addr", cfg.App.Listen))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http listen", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown", zap.Error(err))
	}
	log.Info("bye")
}

// accountProxyResolver 把账号 ID → 代理 URL 的查询串起来。
// 外部看起来就是一个 account.AccountProxyResolver,避免 account 包反向依赖 proxy 包。
type accountProxyResolver struct {
	accSvc   *account.Service
	proxySvc *proxy.Service
}

// ProxyURLForAccount 查账号绑定的代理并解密密码,返回可直接用于 http.ProxyURL 的 URL。
// 任意一步失败都返回 ""(调用方会降级到直连)。
func (r *accountProxyResolver) ProxyURLForAccount(ctx context.Context, accountID uint64) string {
	if r == nil || r.accSvc == nil || r.proxySvc == nil {
		return ""
	}
	b, err := r.accSvc.GetBinding(ctx, accountID)
	if err != nil || b == nil {
		return ""
	}
	p, err := r.proxySvc.Get(ctx, b.ProxyID)
	if err != nil || p == nil || !p.Enabled {
		return ""
	}
	u, err := r.proxySvc.BuildURL(p)
	if err != nil {
		return ""
	}
	return u
}

// ProxyURLByID 按 proxy_id 直接查代理 URL(不经过账号绑定)。
// 供 /accounts/import-tokens 这类"还没有 account_id、但用户已指定代理"的场景使用。
func (r *accountProxyResolver) ProxyURLByID(ctx context.Context, proxyID uint64) string {
	if r == nil || r.proxySvc == nil || proxyID == 0 {
		return ""
	}
	p, err := r.proxySvc.Get(ctx, proxyID)
	if err != nil || p == nil || !p.Enabled {
		return ""
	}
	u, err := r.proxySvc.BuildURL(p)
	if err != nil {
		return ""
	}
	return u
}

// AuthToken 给图片代理端点用:按 accountID 解出 AT / DeviceID / cookies。
// 实现 gateway.ImageAccountResolver。
func (r *accountProxyResolver) AuthToken(ctx context.Context, accountID uint64) (string, string, string, error) {
	if r == nil || r.accSvc == nil {
		return "", "", "", fmt.Errorf("account service not ready")
	}
	a, err := r.accSvc.Get(ctx, accountID)
	if err != nil {
		return "", "", "", err
	}
	at, err := r.accSvc.DecryptAuthToken(a)
	if err != nil {
		return "", "", "", err
	}
	cookies, _ := r.accSvc.DecryptCookies(ctx, accountID)
	return at, a.OAIDeviceID, cookies, nil
}

// ProxyURL 给图片代理端点用:等价于 ProxyURLForAccount。
func (r *accountProxyResolver) ProxyURL(ctx context.Context, accountID uint64) string {
	return r.ProxyURLForAccount(ctx, accountID)
}
