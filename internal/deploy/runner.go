package deploy

import (
	"fmt"
	"log/slog"
	"strings"
)

// Result is the outcome of a deploy run.
type Result struct {
	CertificateID string
}

// Runner executes all targets from deploy.yml in order.
type Runner struct {
	targets      []Target
	tencent      *tencentClients
	defaultAlias string
}

// New loads targets and prepares cloud clients when needed.
func New(targetsFile string, creds CloudCreds, defaultAlias string) (*Runner, error) {
	targets, err := LoadTargets(targetsFile)
	if err != nil {
		return nil, err
	}

	r := &Runner{targets: targets, defaultAlias: defaultAlias}
	if NeedsTencent(targets) {
		clients, err := newTencentClients(creds)
		if err != nil {
			return nil, err
		}
		r.tencent = clients
	}

	slog.Info("deploy targets loaded", "file", targetsFile, "count", len(targets))
	return r, nil
}

// DeployAll runs every target sequentially. Earlier tencent_ssl cert IDs
// are reused by following cdn / clb / apigateway targets unless certificate_id is set.
func (r *Runner) DeployAll(certPEM, keyPEM []byte) (*Result, error) {
	if len(r.targets) == 0 {
		slog.Info("no deploy targets configured, skip")
		return &Result{}, nil
	}

	result := &Result{}
	var errs []string

	for _, t := range r.targets {
		slog.Info("deploy start", "name", t.Name, "type", t.Type)
		if err := r.deployOne(t, certPEM, keyPEM, result); err != nil {
			slog.Error("deploy failed", "name", t.Name, "type", t.Type, "err", err)
			errs = append(errs, fmt.Sprintf("%s(%s): %v", t.Name, t.Type, err))
			continue
		}
		slog.Info("deploy ok", "name", t.Name, "type", t.Type)
	}

	if len(errs) > 0 {
		return result, fmt.Errorf("deploy errors: %s", strings.Join(errs, "; "))
	}
	return result, nil
}

func (r *Runner) deployOne(t Target, certPEM, keyPEM []byte, result *Result) error {
	switch t.Type {
	case TypeTencentSSL:
		if r.tencent == nil {
			return fmt.Errorf("tencent client not initialized")
		}
		id, err := r.tencent.uploadSSL(t, certPEM, keyPEM, r.defaultAlias)
		if err != nil {
			return err
		}
		result.CertificateID = id
		return nil

	case TypeCDN:
		if r.tencent == nil {
			return fmt.Errorf("tencent client not initialized")
		}
		certID := t.CertificateID
		if certID == "" {
			certID = result.CertificateID
		}
		return r.tencent.deployCDN(t, certID)

	case TypeCLB:
		if r.tencent == nil {
			return fmt.Errorf("tencent client not initialized")
		}
		certID := t.CertificateID
		if certID == "" {
			certID = result.CertificateID
		}
		return r.tencent.deployCLB(t, certID)

	case TypeAPIGateway:
		if r.tencent == nil {
			return fmt.Errorf("tencent client not initialized")
		}
		certID := t.CertificateID
		if certID == "" {
			certID = result.CertificateID
		}
		return r.tencent.updateAPIGateway(t, certID)

	case TypeLocal:
		return deployLocal(t, certPEM, keyPEM)

	case TypeSSH:
		return deploySSH(t, certPEM, keyPEM)

	default:
		return fmt.Errorf("unknown type %q", t.Type)
	}
}
