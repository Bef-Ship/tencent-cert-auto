# tencent-cert-auto

基于 **Go + lego** 的腾讯云全站 SSL 证书自动化运维服务（Headless / Docker）。

签发与续期后，按 **一份 `deploy.yml`** 顺序部署到所有目标：腾讯云 SSL、CDN、CLB、API 网关、同机 Nginx、远程轻量/CVM。

## 能力

- DNS-01（DNSPod）申请 / 续期 Let’s Encrypt 泛域名证书
- 每 24h 检测，剩余 ≤30 天自动续期
- **统一部署目标**（`deploy.yml`）
  - `tencent_ssl` — 上传到 SSL 证书平台，拿到 CertificateId
  - `cdn` — **API** 部署到 CDN（`DeployCertificateInstance`）
  - `clb` — **API** 部署到 CLB 监听器（`DeployCertificateInstance`）
  - `apigateway` — **API** 更新 API 网关自定义域名证书
  - `local` — 同机写证书 + reload（不走 SSH）
  - `ssh` — 远程 **真实 SSH/SFTP** + reload（不是腾讯云 API）
- 消息推送：企微 / Slack / Lark
- Docker 常驻、无端口暴露

## 部署方式说明

| 目标类型 | 通道 | 适用场景 |
|---|---|---|
| `tencent_ssl` / `cdn` / `clb` / `apigateway` | 腾讯云 CAM API | 云产品证书绑定 |
| `local` | 本机写文件 | 与本服务同机的 Nginx |
| `ssh` | SSH 私钥 + SFTP | CVM / 轻量上**自建 Nginx** |

`ssh` **不能**改成「腾讯云 API 模式」：云 API 的证书一键部署面向 CDN/CLB 等云产品，不会往机器上的 `/etc/nginx/ssl` 写文件。自建 Nginx 请继续用 `ssh`（或 `local`）。

CDN / CLB 必须走 API：控制台「自动推送」不覆盖**上传的 Let’s Encrypt 证书**。

## 快速部署

```bash
cp .env.example .env
cp deploy.example.yml deploy.yml   # 按环境编辑 targets
mkdir -p cert-data deploy-ssl ssh

docker compose up -d --build
docker logs -f tencent-cert-auto
```

强制续期：`docker exec tencent-cert-auto /app/tencent-cert-auto -once -force`

## deploy.yml 示例

```yaml
targets:
  - name: ssl
    type: tencent_ssl
    alias_prefix: example-wildcard-auto

  - name: cdn-sites
    type: cdn
    domains:
      - www.example.com
      - static.example.com
    # https_billing: "off"   # on|off，默认 off

  - name: clb-https
    type: clb
    # region: ap-shanghai    # 默认 TENCENT_REGION，须与 CLB 地域一致
    load_balancer_id: lb-xxxx
    listener_id: lbl-yyyy
    domain: www.example.com  # 开启 SNI 时必填；未开 SNI / 四层可省略

  - name: apigw-api
    type: apigateway
    service_id: service-xxxx
    domain: api.example.com

  - name: api-local          # 与 cert-auto 同机
    type: local
    cert_path: /deploy/ssl/fullchain.pem
    key_path: /deploy/ssl/privkey.pem
    reload_cmd: "docker exec nginx nginx -s reload"

  - name: develop-api        # 另一台轻量（SSH，非 API）
    type: ssh
    host: 43.142.106.15
    user: root
    private_key: /app/ssh/id_rsa
    cert_path: /etc/nginx/ssl/fullchain.pem
    key_path: /etc/nginx/ssl/privkey.pem
    reload_cmd: "nginx -s reload"
```

- **密钥**放 `.env` / `./ssh`，**目标**放 `deploy.yml`
- `cdn` / `clb` / `apigateway` 默认使用本轮 `tencent_ssl` 返回的 CertificateId
- `local`：同机直接写文件；需要 `docker exec` 时挂载 `docker.sock`

完整字段见 [deploy.example.yml](deploy.example.yml)。

## CAM 最小权限

含 `tencent_ssl` / `cdn` / `clb` / `apigateway` 时需要 `TENCENT_SECRET_ID` / `TENCENT_SECRET_KEY`：

| Action | 用途 |
|---|---|
| `ssl:UploadCertificate` | `tencent_ssl` |
| `ssl:DeployCertificateInstance` | `cdn` / `clb` |
| `apigateway:ModifySubDomain` | `apigateway` |

首次部署若报角色未授权，在控制台 SSL 证书 → 云资源部署中完成一次授权即可。

## 环境变量

见 [.env.example](.env.example)。签发必填：`DP_ID`、`DP_KEY`、`LEGO_EMAIL`、`CERT_DOMAIN`。

### 镜像地址示例

```text
ccr.ccs.tencentyun.com/befship/tencent-cert-auto:latest
ccr.ccs.tencentyun.com/befship/tencent-cert-auto:v1.0.0
```
