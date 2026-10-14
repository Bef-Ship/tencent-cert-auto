package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUnifiedTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.yml")
	content := `
targets:
  - name: ssl
    type: tencent_ssl
    alias_prefix: demo-auto
  - name: cdn-sites
    type: cdn
    domain: www.example.com
    domains:
      - static.example.com
  - name: clb-https
    type: clb
    region: ap-shanghai
    load_balancer_id: lb-aaaa
    listener_id: lbl-bbbb
    domain: www.example.com
    listeners:
      - load_balancer_id: lb-aaaa
        listener_id: lbl-cccc
  - name: apigw
    type: apigateway
    service_id: service-1
    domain: api.example.com
  - name: same-host
    type: local
    cert_path: /data/ssl/fullchain.pem
    key_path: /data/ssl/privkey.pem
    reload_cmd: "nginx -s reload"
  - name: develop
    type: ssh
    host: 1.2.3.4
    user: ubuntu
    private_key: /app/ssh/id_rsa
    cert_path: /etc/nginx/ssl/fullchain.pem
    key_path: /etc/nginx/ssl/privkey.pem
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	targets, err := LoadTargets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 6 {
		t.Fatalf("len=%d", len(targets))
	}
	if !NeedsTencent(targets) {
		t.Fatal("expected NeedsTencent")
	}
	if targets[1].Type != TypeCDN || len(targets[1].Domains) != 2 || targets[1].HTTPSBilling != "off" {
		t.Fatalf("cdn: type=%s domains=%v billing=%s", targets[1].Type, targets[1].Domains, targets[1].HTTPSBilling)
	}
	if targets[2].Type != TypeCLB || len(targets[2].Listeners) != 2 {
		t.Fatalf("clb: type=%s listeners=%v", targets[2].Type, targets[2].Listeners)
	}
	if got := targets[2].Listeners[0].InstanceID(); got != "lb-aaaa|lbl-bbbb|www.example.com" {
		t.Fatalf("clb instance id=%s", got)
	}
	if targets[4].Type != TypeLocal || targets[5].Type != TypeSSH {
		t.Fatalf("types: %s %s", targets[4].Type, targets[5].Type)
	}
}

func TestLoadLegacyHostsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.yml")
	content := `
hosts:
  - name: local-only
    type: local
    cert_path: /c
    key_path: /k
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	targets, err := LoadTargets(path)
	if err != nil || len(targets) != 1 {
		t.Fatalf("err=%v len=%d", err, len(targets))
	}
}
