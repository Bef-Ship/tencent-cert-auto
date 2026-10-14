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
	if len(targets) != 4 {
		t.Fatalf("len=%d", len(targets))
	}
	if !NeedsTencent(targets) {
		t.Fatal("expected NeedsTencent")
	}
	if targets[2].Type != TypeLocal || targets[3].Type != TypeSSH {
		t.Fatalf("types: %s %s", targets[2].Type, targets[3].Type)
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
