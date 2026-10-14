package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration loaded from environment variables.
// Deploy destinations (ssl / apigw / local / ssh) live in deploy.yml — not here.
type Config struct {
	// DNSPod
	DPID  string
	DPKey string

	// Tencent Cloud CAM (used by tencent_ssl / apigateway targets)
	TencentSecretID  string
	TencentSecretKey string
	TencentRegion    string

	// Message push (WeCom / Slack / Lark)
	NotifyWebhook  string
	NotifyProvider string // auto|wecom|slack|lark

	// Certificate
	Domain    string // e.g. *.example.com
	Email     string
	CertDir   string
	RenewDays int

	// Unified deploy targets file
	DeployTargetsFile string

	// Scheduler
	CheckInterval time.Duration
	ForceRenew    bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		DPID:              strings.TrimSpace(os.Getenv("DP_ID")),
		DPKey:             strings.TrimSpace(os.Getenv("DP_KEY")),
		TencentSecretID:   strings.TrimSpace(os.Getenv("TENCENT_SECRET_ID")),
		TencentSecretKey:  strings.TrimSpace(os.Getenv("TENCENT_SECRET_KEY")),
		TencentRegion:     envOr("TENCENT_REGION", "ap-shanghai"),
		NotifyWebhook:     firstEnv("NOTIFY_WEBHOOK", "WECOM_WEBHOOK"),
		NotifyProvider:    envOr("NOTIFY_PROVIDER", "auto"),
		Domain:            envOr("CERT_DOMAIN", "*.example.com"),
		Email:             strings.TrimSpace(os.Getenv("LEGO_EMAIL")),
		CertDir:           envOr("CERT_DIR", "/app/cert-data"),
		RenewDays:         envInt("RENEW_DAYS", 30),
		DeployTargetsFile: firstEnv("DEPLOY_TARGETS_FILE", "DEPLOY_HOSTS_FILE"),
		CheckInterval:     envDuration("CHECK_INTERVAL", 24*time.Hour),
		ForceRenew:        envBool("FORCE_RENEW", false),
	}
	if cfg.DeployTargetsFile == "" {
		cfg.DeployTargetsFile = "/app/deploy.yml"
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks required fields for certificate issuance.
// Tencent secrets are validated later only if deploy.yml needs them.
func (c *Config) Validate() error {
	required := map[string]string{
		"DP_ID":       c.DPID,
		"DP_KEY":      c.DPKey,
		"LEGO_EMAIL":  c.Email,
		"CERT_DOMAIN": c.Domain,
	}
	var missing []string
	for k, v := range required {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Domains returns the ACME domain list (wildcard + apex when applicable).
func (c *Config) Domains() []string {
	domains := []string{c.Domain}
	if strings.HasPrefix(c.Domain, "*.") {
		apex := strings.TrimPrefix(c.Domain, "*.")
		if apex != "" {
			domains = append(domains, apex)
		}
	}
	return domains
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
