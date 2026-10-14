package deploy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Target types — all destinations live in one deploy.yml.
const (
	TypeTencentSSL = "tencent_ssl" // upload to Tencent SSL certificate platform
	TypeCDN        = "cdn"         // deploy cert to CDN domains via API
	TypeCLB        = "clb"         // deploy cert to CLB listeners via API
	TypeAPIGateway = "apigateway"  // bind cert to API Gateway custom domain
	TypeLocal      = "local"       // same host: write files + reload
	TypeSSH        = "ssh"         // remote host: SFTP + reload (not Tencent API)
)

// CLBListener is one CLB HTTPS listener binding for DeployCertificateInstance.
// Format: LoadBalancerId|ListenerId[|Domain] — Domain required when SNI is on.
type CLBListener struct {
	LoadBalancerID string `yaml:"load_balancer_id"`
	ListenerID     string `yaml:"listener_id"`
	Domain         string `yaml:"domain"` // optional: SNI domain on L7 listener
}

// Target is one deploy destination.
type Target struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // tencent_ssl | cdn | clb | apigateway | local | ssh

	// tencent_ssl
	AliasPrefix string `yaml:"alias_prefix"`

	// cdn / apigateway / clb shared
	Domain string `yaml:"domain"`
	// Domains optional multi-domain list for cdn (merged with domain)
	Domains []string `yaml:"domains"`
	// CertificateID optional; empty uses last tencent_ssl upload in this run
	CertificateID string `yaml:"certificate_id"`
	// Region optional override (CLB requires region matching the LB)
	Region string `yaml:"region"`

	// cdn: HTTPS billing switch passed to DeployCertificateInstance (on|off)
	HTTPSBilling string `yaml:"https_billing"`

	// clb: single listener shorthand and/or listeners list
	LoadBalancerID string        `yaml:"load_balancer_id"`
	ListenerID     string        `yaml:"listener_id"`
	Listeners      []CLBListener `yaml:"listeners"`

	// apigateway
	ServiceID string `yaml:"service_id"`
	NetType   string `yaml:"net_type"` // default OUTER
	Protocol  string `yaml:"protocol"` // default https

	// local / ssh shared paths
	CertPath  string `yaml:"cert_path"`
	KeyPath   string `yaml:"key_path"`
	ReloadCmd string `yaml:"reload_cmd"`

	// ssh — real SSH/SFTP to CVM/Lighthouse nginx; not Tencent Cloud API
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
	case TypeCDN:
		t.Domains = mergeDomains(t.Domain, t.Domains)
		if len(t.Domains) == 0 {
			return fmt.Errorf("cdn requires domain or domains")
		}
		t.Domain = t.Domains[0]
		switch strings.ToLower(strings.TrimSpace(t.HTTPSBilling)) {
		case "", "off":
			t.HTTPSBilling = "off"
		case "on":
			t.HTTPSBilling = "on"
		default:
			return fmt.Errorf("cdn https_billing must be on or off")
		}
		return nil
	case TypeCLB:
		t.Listeners = mergeCLBListeners(t)
		if len(t.Listeners) == 0 {
			return fmt.Errorf("clb requires listeners or load_balancer_id+listener_id")
		}
		for i, l := range t.Listeners {
			if l.LoadBalancerID == "" || l.ListenerID == "" {
				return fmt.Errorf("clb listeners[%d] requires load_balancer_id and listener_id", i)
			}
		}
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
		return fmt.Errorf("unknown type %q (want tencent_ssl|cdn|clb|apigateway|local|ssh)", t.Type)
	}
}

func mergeCLBListeners(t *Target) []CLBListener {
	out := make([]CLBListener, 0, len(t.Listeners)+1)
	if t.LoadBalancerID != "" || t.ListenerID != "" {
		out = append(out, CLBListener{
			LoadBalancerID: strings.TrimSpace(t.LoadBalancerID),
			ListenerID:     strings.TrimSpace(t.ListenerID),
			Domain:         strings.TrimSpace(t.Domain),
		})
	}
	for _, l := range t.Listeners {
		out = append(out, CLBListener{
			LoadBalancerID: strings.TrimSpace(l.LoadBalancerID),
			ListenerID:     strings.TrimSpace(l.ListenerID),
			Domain:         strings.TrimSpace(l.Domain),
		})
	}
	return out
}

// InstanceID builds DeployCertificateInstance InstanceIdList entry for CLB.
func (l CLBListener) InstanceID() string {
	if l.Domain != "" {
		return l.LoadBalancerID + "|" + l.ListenerID + "|" + l.Domain
	}
	return l.LoadBalancerID + "|" + l.ListenerID
}

func mergeDomains(single string, list []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(list)+1)
	add := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		if _, ok := seen[d]; ok {
			return
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	add(single)
	for _, d := range list {
		add(d)
	}
	return out
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
		switch t.Type {
		case TypeTencentSSL, TypeCDN, TypeCLB, TypeAPIGateway:
			return true
		}
	}
	return false
}
