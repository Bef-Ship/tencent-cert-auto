package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/befship/tencent-cert-auto/internal/cert"
	"github.com/befship/tencent-cert-auto/internal/config"
	"github.com/befship/tencent-cert-auto/internal/deploy"
	"github.com/befship/tencent-cert-auto/internal/notify"
)

// App is the headless certificate automation service.
type App struct {
	cfg      *config.Config
	manager  *cert.Manager
	deployer *deploy.Runner
	notifier *notify.Notifier
}

// New constructs the application from config.
func New(cfg *config.Config) (*App, error) {
	manager, err := cert.NewManager(
		cfg.Domains(),
		cfg.Email,
		cfg.CertDir,
		cfg.RenewDays,
		cfg.DPID,
		cfg.DPKey,
	)
	if err != nil {
		return nil, err
	}

	deployer, err := deploy.New(
		cfg.DeployTargetsFile,
		deploy.CloudCreds{
			SecretID:  cfg.TencentSecretID,
			SecretKey: cfg.TencentSecretKey,
			Region:    cfg.TencentRegion,
		},
		aliasPrefix(cfg.Domain),
	)
	if err != nil {
		return nil, fmt.Errorf("init deploy: %w", err)
	}

	return &App{
		cfg:      cfg,
		manager:  manager,
		deployer: deployer,
		notifier: notify.New(cfg.NotifyWebhook, cfg.NotifyProvider),
	}, nil
}

// Run blocks until context cancel / signal, checking certificates on an interval.
func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("tencent-cert-auto started",
		"domain", a.cfg.Domain,
		"renew_days", a.cfg.RenewDays,
		"interval", a.cfg.CheckInterval.String(),
		"cert_dir", a.cfg.CertDir,
	)

	a.runOnce(ctx)

	ticker := time.NewTicker(a.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return nil
		case <-ticker.C:
			a.runOnce(ctx)
		}
	}
}

// RunOnce executes a single check/renew/deploy cycle.
func (a *App) RunOnce(ctx context.Context) error {
	return a.runOnce(ctx)
}

func (a *App) runOnce(ctx context.Context) error {
	slog.Info("======== certificate check start ========",
		"time", time.Now().Format("2006-01-02 15:04:05"),
	)

	need, existing, err := a.manager.NeedRenew(a.cfg.ForceRenew)
	if err != nil {
		a.fail("证书状态检测", err)
		return err
	}
	if !need {
		slog.Info("certificate still valid, skip renew")
		slog.Info("======== certificate check done ========")
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	bundle, err := a.manager.Obtain(existing)
	if err != nil {
		a.fail("证书签发/续期", err)
		return err
	}

	result, err := a.deployer.DeployAll(bundle.Certificate, bundle.PrivateKey)
	if err != nil {
		a.fail("证书部署", err)
		return err
	}

	certID := ""
	if result != nil {
		certID = result.CertificateID
	}
	a.notifier.Success(a.cfg.Domain, certID)
	slog.Info("renew & deploy succeeded", "certificate_id", certID)
	slog.Info("======== certificate check done ========")

	a.cfg.ForceRenew = false
	return nil
}

func (a *App) fail(stage string, err error) {
	slog.Error(stage+" failed", "err", err)
	a.notifier.Failure(stage, err)
}

func aliasPrefix(domain string) string {
	d := strings.ReplaceAll(domain, "*", "wildcard")
	d = strings.ReplaceAll(d, ".", "-")
	return d + "-auto"
}

// SetupLogger configures JSON or text slog based on LOG_FORMAT.
func SetupLogger() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// MustLoad is a helper for main.
func MustLoad() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}
