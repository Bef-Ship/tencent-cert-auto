package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadAndDomains(t *testing.T) {
	env := map[string]string{
		"DP_ID":          "1",
		"DP_KEY":         "k",
		"LEGO_EMAIL":     "ops@example.com",
		"CERT_DOMAIN":    "*.example.com",
		"CHECK_INTERVAL": "1h",
		"RENEW_DAYS":     "20",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	_ = os.Unsetenv("FORCE_RENEW")
	_ = os.Unsetenv("DEPLOY_TARGETS_FILE")
	_ = os.Unsetenv("DEPLOY_HOSTS_FILE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RenewDays != 20 {
		t.Fatalf("RenewDays=%d", cfg.RenewDays)
	}
	if cfg.CheckInterval != time.Hour {
		t.Fatalf("CheckInterval=%v", cfg.CheckInterval)
	}
	got := cfg.Domains()
	if len(got) != 2 || got[0] != "*.example.com" || got[1] != "example.com" {
		t.Fatalf("Domains=%v", got)
	}
}

func TestValidateMissing(t *testing.T) {
	t.Setenv("DP_ID", "")
	t.Setenv("DP_KEY", "")
	t.Setenv("TENCENT_SECRET_ID", "")
	t.Setenv("TENCENT_SECRET_KEY", "")
	t.Setenv("LEGO_EMAIL", "")
	t.Setenv("CERT_DOMAIN", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}
