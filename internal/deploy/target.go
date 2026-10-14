package deploy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Target types — all destinations live in one deploy.yml.
const (
	TypeTencentSSL  = "tencent_ssl" // upload to Tencent SSL platform (CDN auto-push)
	TypeAPIGateway  = "apigateway"  // bind cert to API Gateway custom domain
	TypeLocal       = "local"       // same host: write files + reload
	TypeSSH         = "ssh"         // remote host: SFTP + reload
)

// Target is one deploy destination.
type Target struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // tencent_ssl | apigateway | local | ssh

	// tencent_ssl
	AliasPrefix string `yaml:"alias_prefix"`

	// apigateway
	ServiceID string `yaml:"service_id"`
	Domain    string `yaml:"domain"`
	// CertificateID optional; empty uses last tencent_ssl upload in this run
	CertificateID string `yaml:"certificate_id"`
	NetType       string `yaml:"net_type"` // default OUTER
	Protocol      string `yaml:"protocol"` // default https

	// local / ssh shared paths
	CertPath  string `yaml:"cert_path"`
	KeyPath   string `yaml:"key_path"`
	ReloadCmd string `yaml:"reload_cmd"`

	// ssh
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	User       string `yaml:"user"`
	PrivateKey string `yaml:"private_key"`
	Passphrase string `yaml:"passphrase"`
	KnownHosts string `yaml:"known_hosts"`
}

type fileSchema struct {
	Targets []Target `yaml:"targets"`
	// Hosts is accepted as an alias of targets for older deploy-hosts.yml.
	Hosts []Target `yaml:"hosts"`
}

// LoadTargets reads unified deploy YAML. Missing file → empty list.
func LoadTargets(path string) ([]Target, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var doc fileSchema
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	list := doc.Targets
	if len(list) == 0 {
		list = doc.Hosts
	}

	for i := range list {
		if err := list[i].normalize(); err != nil {
			return nil, fmt.Errorf("targets[%d] %s: %w", i, list[i].Name, err)
		}
	}
	return list, nil
}

func (t *Target) normalize() error {
	t.Type = strings.ToLower(strings.TrimSpace(t.Type))
	if t.Type == "" {
		// legacy hostdeploy entries without type
		if t.Host != "" && !isLocalHost(t.Host) {
			t.Type = TypeSSH
		} else {
			t.Type = TypeLocal
		}
	}
	if t.Name == "" {
		t.Name = t.Type
		if t.Host != "" {
			t.Name = t.Host
		}
	}

	switch t.Type {
	case TypeTencentSSL:
		return nil
	case TypeAPIGateway:
		if t.ServiceID == "" || t.Domain == "" {
			return fmt.Errorf("apigateway requires service_id and domain")
		}
		if t.NetType == "" {
			t.NetType = "OUTER"
		}
		if t.Protocol == "" {
			t.Protocol = "https"
		}
		return nil
	case TypeLocal:
		if t.CertPath == "" || t.KeyPath == "" {
			return fmt.Errorf("local requires cert_path and key_path")
		}
		return nil
	case TypeSSH:
		if t.Host == "" {
			return fmt.Errorf("ssh requires host")
		}
		if t.PrivateKey == "" {
			return fmt.Errorf("ssh requires private_key")
		}
		if t.CertPath == "" || t.KeyPath == "" {
			return fmt.Errorf("ssh requires cert_path and key_path")
		}
		if t.Port <= 0 {
			t.Port = 22
		}
		if t.User == "" {
			t.User = "root"
		}
		return nil
	default:
		return fmt.Errorf("unknown type %q (want tencent_ssl|apigateway|local|ssh)", t.Type)
	}
}

func isLocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	default:
		return false
	}
}

// NeedsTencent reports whether any target talks to Tencent Cloud APIs.
func NeedsTencent(targets []Target) bool {
	for _, t := range targets {
		if t.Type == TypeTencentSSL || t.Type == TypeAPIGateway {
			return true
		}
	}
	return false
}
