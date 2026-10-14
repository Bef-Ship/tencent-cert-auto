package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/registration"
)

// Bundle holds PEM-encoded certificate material and metadata.
type Bundle struct {
	Domain      string
	Certificate []byte
	PrivateKey  []byte
	CertPath    string
	KeyPath     string
	NotAfter    time.Time
}

// Manager wraps go-acme/lego for issue/renew against DNSPod DNS-01.
type Manager struct {
	domains   []string
	email     string
	certDir   string
	renewDays int
	client    *lego.Client
}

// NewManager builds a lego client with DNSPod DNS-01 challenge.
func NewManager(domains []string, email, certDir string, renewDays int, dpID, dpKey string) (*Manager, error) {
	accountsDir := filepath.Join(certDir, "accounts")
	user, err := loadOrCreateUser(accountsDir, email)
	if err != nil {
		return nil, fmt.Errorf("load acme user: %w", err)
	}

	cfg := lego.NewConfig(user)
	cfg.Certificate.KeyType = certcrypto.RSA2048
	cfg.CADirURL = lego.LEDirectoryProduction

	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new lego client: %w", err)
	}

	// lego dnspod expects LoginToken = "id,token"
	dnsCfg := dnspod.NewDefaultConfig()
	dnsCfg.LoginToken = fmt.Sprintf("%s,%s", dpID, dpKey)
	provider, err := dnspod.NewDNSProviderConfig(dnsCfg)
	if err != nil {
		return nil, fmt.Errorf("dnspod provider: %w", err)
	}
	if err := client.Challenge.SetDNS01Provider(provider, dns01.AddRecursiveNameservers([]string{
		"1.1.1.1:53",
		"8.8.8.8:53",
	})); err != nil {
		return nil, fmt.Errorf("set dns01 provider: %w", err)
	}

	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("acme register: %w", err)
		}
		user.Registration = reg
		if err := user.saveMeta(); err != nil {
			return nil, fmt.Errorf("save account meta: %w", err)
		}
		slog.Info("acme account registered", "email", email)
	}

	if err := os.MkdirAll(filepath.Join(certDir, "certificates"), 0o700); err != nil {
		return nil, err
	}

	return &Manager{
		domains:   domains,
		email:     email,
		certDir:   certDir,
		renewDays: renewDays,
		client:    client,
	}, nil
}

// NeedRenew reports whether a new issue or renew is required.
func (m *Manager) NeedRenew(force bool) (bool, *Bundle, error) {
	bundle, err := m.LoadExisting()
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("no existing certificate, will issue")
			return true, nil, nil
		}
		return false, nil, err
	}

	remain := time.Until(bundle.NotAfter)
	slog.Info("certificate status",
		"domain", bundle.Domain,
		"not_after", bundle.NotAfter.Format(time.RFC3339),
		"remain_days", int(remain.Hours()/24),
	)

	if force {
		return true, bundle, nil
	}
	if remain <= time.Duration(m.renewDays)*24*time.Hour {
		return true, bundle, nil
	}
	return false, bundle, nil
}

// Obtain issues a new certificate or renews an existing one.
func (m *Manager) Obtain(existing *Bundle) (*Bundle, error) {
	primary := m.domains[0]
	req := certificate.ObtainRequest{
		Domains: m.domains,
		Bundle:  true,
	}

	var (
		res *certificate.Resource
		err error
	)

	if existing != nil {
		res, err = m.client.Certificate.RenewWithOptions(certificate.Resource{
			Domain:      existing.Domain,
			Certificate: existing.Certificate,
			PrivateKey:  existing.PrivateKey,
		}, &certificate.RenewOptions{
			Bundle: true,
		})
		if err != nil {
			slog.Warn("renew failed, fallback to obtain", "err", err)
			res, err = m.client.Certificate.Obtain(req)
		}
	} else {
		res, err = m.client.Certificate.Obtain(req)
	}
	if err != nil {
		return nil, fmt.Errorf("obtain/renew certificate: %w", err)
	}

	bundle, err := m.persist(res)
	if err != nil {
		return nil, err
	}
	slog.Info("certificate obtained", "domain", primary, "not_after", bundle.NotAfter.Format(time.RFC3339))
	return bundle, nil
}

// LoadExisting reads the on-disk certificate for the primary domain.
func (m *Manager) LoadExisting() (*Bundle, error) {
	base := sanitizeDomain(m.domains[0])
	certPath := filepath.Join(m.certDir, "certificates", base+".crt")
	keyPath := filepath.Join(m.certDir, "certificates", base+".key")

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	notAfter, err := parseNotAfter(certPEM)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Domain:      m.domains[0],
		Certificate: certPEM,
		PrivateKey:  keyPEM,
		CertPath:    certPath,
		KeyPath:     keyPath,
		NotAfter:    notAfter,
	}, nil
}

func (m *Manager) persist(res *certificate.Resource) (*Bundle, error) {
	base := sanitizeDomain(res.Domain)
	dir := filepath.Join(m.certDir, "certificates")
	certPath := filepath.Join(dir, base+".crt")
	keyPath := filepath.Join(dir, base+".key")
	issuerPath := filepath.Join(dir, base+".issuer.crt")
	jsonPath := filepath.Join(dir, base+".json")

	if err := os.WriteFile(certPath, res.Certificate, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, res.PrivateKey, 0o600); err != nil {
		return nil, err
	}
	if len(res.IssuerCertificate) > 0 {
		_ = os.WriteFile(issuerPath, res.IssuerCertificate, 0o644)
	}
	_ = os.WriteFile(jsonPath, []byte(fmt.Sprintf(`{"domain":%q,"certUrl":%q,"certStableUrl":%q}`,
		res.Domain, res.CertURL, res.CertStableURL)), 0o644)

	notAfter, err := parseNotAfter(res.Certificate)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Domain:      res.Domain,
		Certificate: res.Certificate,
		PrivateKey:  res.PrivateKey,
		CertPath:    certPath,
		KeyPath:     keyPath,
		NotAfter:    notAfter,
	}, nil
}

func sanitizeDomain(domain string) string {
	return strings.ReplaceAll(domain, "*", "_")
}

func parseNotAfter(certPEM []byte) (time.Time, error) {
	var first *x509.Certificate
	rest := certPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		first = c
		break
	}
	if first != nil {
		return first.NotAfter, nil
	}

	// Fallback: tls.X509KeyPair style leaf parse
	certs, err := tls.X509KeyPair(certPEM, certPEM)
	if err == nil && len(certs.Certificate) > 0 {
		c, err := x509.ParseCertificate(certs.Certificate[0])
		if err == nil {
			return c.NotAfter, nil
		}
	}
	return time.Time{}, fmt.Errorf("no certificate found in PEM")
}
