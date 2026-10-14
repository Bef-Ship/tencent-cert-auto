package deploy

import (
	"fmt"
	"log/slog"
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
	ssl   *ssl.Client
	apigw *apigateway.Client
}

func newTencentClients(c CloudCreds) (*tencentClients, error) {
	if c.SecretID == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("TENCENT_SECRET_ID/KEY required for tencent_ssl/apigateway targets")
	}
	if c.Region == "" {
		c.Region = "ap-shanghai"
	}
	cred := common.NewCredential(c.SecretID, c.SecretKey)

	sslProfile := profile.NewClientProfile()
	sslProfile.HttpProfile.Endpoint = "ssl.tencentcloudapi.com"
	sslClient, err := ssl.NewClient(cred, c.Region, sslProfile)
	if err != nil {
		return nil, fmt.Errorf("ssl client: %w", err)
	}

	apigwProfile := profile.NewClientProfile()
	apigwProfile.HttpProfile.Endpoint = "apigateway.tencentcloudapi.com"
	apigwClient, err := apigateway.NewClient(cred, c.Region, apigwProfile)
	if err != nil {
		return nil, fmt.Errorf("apigw client: %w", err)
	}

	return &tencentClients{ssl: sslClient, apigw: apigwClient}, nil
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
