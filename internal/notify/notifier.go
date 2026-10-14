package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider identifies the webhook message format.
type Provider string

const (
	ProviderAuto  Provider = "auto"
	ProviderWeCom Provider = "wecom"
	ProviderSlack Provider = "slack"
	ProviderLark  Provider = "lark"
)

// Notifier sends text alerts to WeCom / Slack / Lark webhooks.
type Notifier struct {
	webhook  string
	provider Provider
	client   *http.Client
}

// New creates a notifier. Empty webhook disables sending.
// provider: auto|wecom|slack|lark — auto detects from webhook host.
func New(webhook, provider string) *Notifier {
	p := Provider(strings.ToLower(strings.TrimSpace(provider)))
	if p == "" {
		p = ProviderAuto
	}
	n := &Notifier{
		webhook:  strings.TrimSpace(webhook),
		provider: p,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
	if n.provider == ProviderAuto && n.webhook != "" {
		n.provider = detectProvider(n.webhook)
	}
	return n
}

// Success notifies a successful renew/deploy.
func (n *Notifier) Success(domain, certID string) {
	msg := fmt.Sprintf("✅【SSL证书自动续期成功】\n域名：%s\n证书ID：%s\n更新时间：%s",
		domain, certID, time.Now().Format("2006-01-02 15:04:05"))
	n.Send(msg)
}

// Failure notifies an error.
func (n *Notifier) Failure(stage string, err error) {
	msg := fmt.Sprintf("❌【SSL证书异常】%s失败：%v\n时间：%s",
		stage, err, time.Now().Format("2006-01-02 15:04:05"))
	n.Send(msg)
}

// Send posts a plain text message using the configured provider format.
func (n *Notifier) Send(content string) {
	if n.webhook == "" {
		slog.Debug("notify webhook empty, skip", "content", content)
		return
	}

	body, err := buildPayload(n.provider, content)
	if err != nil {
		slog.Error("notify marshal failed", "provider", n.provider, "err", err)
		return
	}

	resp, err := n.client.Post(n.webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("notify post failed", "provider", n.provider, "err", err)
		return
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		slog.Error("notify non-2xx", "provider", n.provider, "status", resp.StatusCode, "body", string(raw))
		return
	}
	slog.Info("notify sent", "provider", n.provider, "status", resp.StatusCode)
}

func buildPayload(provider Provider, content string) ([]byte, error) {
	switch provider {
	case ProviderSlack:
		return json.Marshal(map[string]string{"text": content})
	case ProviderLark:
		return json.Marshal(map[string]any{
			"msg_type": "text",
			"content":  map[string]string{"text": content},
		})
	case ProviderWeCom, ProviderAuto:
		fallthrough
	default:
		// default WeCom-compatible payload
		return json.Marshal(map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		})
	}
}

func detectProvider(webhook string) Provider {
	u, err := url.Parse(webhook)
	if err != nil {
		return ProviderWeCom
	}
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "qyapi.weixin.qq.com"):
		return ProviderWeCom
	case strings.Contains(host, "hooks.slack.com"), strings.Contains(host, "slack.com"):
		return ProviderSlack
	case strings.Contains(host, "open.feishu.cn"),
		strings.Contains(host, "open.larksuite.com"),
		strings.Contains(host, "feishu.cn"),
		strings.Contains(host, "larksuite.com"):
		return ProviderLark
	default:
		return ProviderWeCom
	}
}
