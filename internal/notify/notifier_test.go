package notify

import "testing"

func TestDetectProvider(t *testing.T) {
	cases := []struct {
		url  string
		want Provider
	}{
		{"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=1", ProviderWeCom},
		{"https://hooks.slack.com/services/T/B/X", ProviderSlack},
		{"https://open.feishu.cn/open-apis/bot/v2/hook/xxx", ProviderLark},
		{"https://open.larksuite.com/open-apis/bot/v2/hook/xxx", ProviderLark},
		{"https://example.com/hook", ProviderWeCom},
	}
	for _, c := range cases {
		if got := detectProvider(c.url); got != c.want {
			t.Fatalf("%s: got %s want %s", c.url, got, c.want)
		}
	}
}

func TestBuildPayload(t *testing.T) {
	for _, p := range []Provider{ProviderWeCom, ProviderSlack, ProviderLark} {
		b, err := buildPayload(p, "hello")
		if err != nil || len(b) == 0 {
			t.Fatalf("provider %s: %v %s", p, err, b)
		}
	}
}
