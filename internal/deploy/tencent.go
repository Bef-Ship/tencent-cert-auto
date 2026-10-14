package deploy

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	apigateway "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/apigateway/v20180808"
	ssl "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ssl/v20191205"
)

// CloudCreds holds Tencent CAM credentials (from env, not YAML).
type CloudCreds struct {
	SecretID  string
	SecretKey string
	Region    string
}

type tencentClients struct {
	ssl    *ssl.Client
	apigw  *apigateway.Client
	cred   *common.Credential
	region string
}

func newTencentClients(c CloudCreds) (*tencentClients, error) {
	if c.SecretID == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("TENCENT_SECRET_ID/KEY required for tencent_ssl/cdn/clb/apigateway targets")
	}
	if c.Region == "" {
		c.Region = "ap-shanghai"
	}
	cred := common.NewCredential(c.SecretID, c.SecretKey)

	sslClient, err := newSSLClient(cred, c.Region)
	if err != nil {
		return nil, err
	}

	apigwProfile := profile.NewClientProfile()
	apigwProfile.HttpProfile.Endpoint = "apigateway.tencentcloudapi.com"
	apigwClient, err := apigateway.NewClient(cred, c.Region, apigwProfile)
	if err != nil {
		return nil, fmt.Errorf("apigw client: %w", err)
	}

	return &tencentClients{ssl: sslClient, apigw: apigwClient, cred: cred, region: c.Region}, nil
}

func newSSLClient(cred *common.Credential, region string) (*ssl.Client, error) {
	sslProfile := profile.NewClientProfile()
	sslProfile.HttpProfile.Endpoint = "ssl.tencentcloudapi.com"
	sslClient, err := ssl.NewClient(cred, region, sslProfile)
	if err != nil {
		return nil, fmt.Errorf("ssl client: %w", err)
	}
	return sslClient, nil
}

func (c *tencentClients) sslForRegion(region string) (*ssl.Client, error) {
	region = strings.TrimSpace(region)
	if region == "" || region == c.region {
		return c.ssl, nil
	}
	return newSSLClient(c.cred, region)
}

func (c *tencentClients) uploadSSL(t Target, certPEM, keyPEM []byte, defaultAlias string) (string, error) {
	alias := t.AliasPrefix
	if alias == "" {
		alias = defaultAlias
	}
	alias = fmt.Sprintf("%s-%s", alias, time.Now().Format("20060102"))

	req := ssl.NewUploadCertificateRequest()
	req.CertificatePublicKey = common.StringPtr(string(certPEM))
	req.CertificatePrivateKey = common.StringPtr(string(keyPEM))
	req.Alias = common.StringPtr(alias)
	req.Repeatable = common.BoolPtr(true)

	resp, err := c.ssl.UploadCertificate(req)
	if err != nil {
		return "", fmt.Errorf("UploadCertificate: %w", err)
	}
	if resp.Response == nil || resp.Response.CertificateId == nil {
		return "", fmt.Errorf("UploadCertificate: empty CertificateId")
	}
	certID := *resp.Response.CertificateId
	slog.Info("tencent_ssl uploaded", "name", t.Name, "certificate_id", certID, "alias", alias)
	return certID, nil
}

func (c *tencentClients) deployCDN(t Target, certID string) error {
	if certID == "" {
		return fmt.Errorf("cdn %s: no certificate_id (run tencent_ssl first or set certificate_id)", t.Name)
	}
	if len(t.Domains) == 0 {
		return fmt.Errorf("cdn %s: no domains", t.Name)
	}

	instances := make([]*string, 0, len(t.Domains))
	for _, d := range t.Domains {
		// CDN InstanceIdList format: Domain|https_billing (on|off)
		item := fmt.Sprintf("%s|%s", d, t.HTTPSBilling)
		instances = append(instances, common.StringPtr(item))
	}

	return c.deployCertificateInstance(t.Name, "cdn", certID, c.region, instances, map[string]any{
		"domains":        t.Domains,
		"https_billing":  t.HTTPSBilling,
	})
}

func (c *tencentClients) deployCLB(t Target, certID string) error {
	if certID == "" {
		return fmt.Errorf("clb %s: no certificate_id (run tencent_ssl first or set certificate_id)", t.Name)
	}
	if len(t.Listeners) == 0 {
		return fmt.Errorf("clb %s: no listeners", t.Name)
	}

	instances := make([]*string, 0, len(t.Listeners))
	ids := make([]string, 0, len(t.Listeners))
	for _, l := range t.Listeners {
		id := l.InstanceID()
		ids = append(ids, id)
		instances = append(instances, common.StringPtr(id))
	}

	region := t.Region
	if region == "" {
		region = c.region
	}
	return c.deployCertificateInstance(t.Name, "clb", certID, region, instances, map[string]any{
		"instances": ids,
		"region":    region,
	})
}

func (c *tencentClients) deployCertificateInstance(
	name, resourceType, certID, region string,
	instances []*string,
	extra map[string]any,
) error {
	client, err := c.sslForRegion(region)
	if err != nil {
		return err
	}

	req := ssl.NewDeployCertificateInstanceRequest()
	req.CertificateId = common.StringPtr(certID)
	req.InstanceIdList = instances
	req.ResourceType = common.StringPtr(resourceType)
	req.IsCache = common.Uint64Ptr(0)

	resp, err := client.DeployCertificateInstance(req)
	if err != nil {
		return fmt.Errorf("DeployCertificateInstance(%s): %w", resourceType, err)
	}

	recordID := uint64(0)
	deployStatus := int64(-1)
	reqID := ""
	if resp.Response != nil {
		if resp.Response.DeployRecordId != nil {
			recordID = *resp.Response.DeployRecordId
		}
		if resp.Response.DeployStatus != nil {
			deployStatus = *resp.Response.DeployStatus
		}
		if resp.Response.RequestId != nil {
			reqID = *resp.Response.RequestId
		}
	}

	attrs := []any{
		"name", name,
		"resource_type", resourceType,
		"certificate_id", certID,
		"deploy_record_id", recordID,
		"deploy_status", deployStatus, // 1=created, 0=existing in-progress
		"request_id", reqID,
	}
	for k, v := range extra {
		attrs = append(attrs, k, v)
	}
	slog.Info(resourceType+" deploy submitted", attrs...)
	return nil
}

func (c *tencentClients) updateAPIGateway(t Target, certID string) error {
	if certID == "" {
		return fmt.Errorf("apigateway %s: no certificate_id (run tencent_ssl first or set certificate_id)", t.Name)
	}

	req := apigateway.NewModifySubDomainRequest()
	req.ServiceId = common.StringPtr(t.ServiceID)
	req.SubDomain = common.StringPtr(t.Domain)
	req.CertificateId = common.StringPtr(certID)
	req.IsDefaultMapping = common.BoolPtr(true)
	req.Protocol = common.StringPtr(t.Protocol)
	req.NetType = common.StringPtr(t.NetType)

	resp, err := c.apigw.ModifySubDomain(req)
	if err != nil {
		return fmt.Errorf("ModifySubDomain: %w", err)
	}
	reqID := ""
	if resp.Response != nil && resp.Response.RequestId != nil {
		reqID = *resp.Response.RequestId
	}
	slog.Info("apigateway updated",
		"name", t.Name,
		"service_id", t.ServiceID,
		"domain", t.Domain,
		"certificate_id", certID,
		"request_id", reqID,
	)
	return nil
}
