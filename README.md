# tencent-cert-auto

基于 **Go + lego** 的腾讯云全站 SSL 证书自动化运维服务（Headless / Docker）。

签发与续期后，按 **一份 `deploy.yml`** 顺序部署到所有目标：腾讯云 SSL、API 网关、同机 Nginx、远程轻量/CVM。

## 能力

- DNS-01（DNSPod）申请 / 续期 Let’s Encrypt 泛域名证书
- 每 24h 检测，剩余 ≤30 天自动续期
- **统一部署目标**（`deploy.yml`）
  - `tencent_ssl` — 上传 SSL（CDN 依赖控制台自动推送）
  - `apigateway` — 更新 API 网关自定义域名证书
  - `local` — 同机写证书 + reload（不走 SSH）
  - `ssh` — 远程 SFTP + reload
- 消息推送：企微 / Slack / Lark
- Docker 常驻、无端口暴露

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

  - name: apigw-api
    type: apigateway
    service_id: service-xxxx
    domain: api.example.com

  - name: api-local          # 与 cert-auto 同机
    type: local
    cert_path: /deploy/ssl/fullchain.pem
    key_path: /deploy/ssl/privkey.pem
    reload_cmd: "docker exec nginx nginx -s reload"

  - name: develop-api        # 另一台轻量
    type: ssh
    host: 43.142.106.15
    user: root
    private_key: /app/ssh/id_rsa
    cert_path: /etc/nginx/ssl/fullchain.pem
    key_path: /etc/nginx/ssl/privkey.pem
    reload_cmd: "nginx -s reload"
```

- **密钥**放 `.env` / `./ssh`，**目标**放 `deploy.yml`
- `apigateway` 默认使用本轮 `tencent_ssl` 返回的 CertificateId
- `local`：同机直接写文件；需要 `docker exec` 时挂载 `docker.sock`

完整字段见 [deploy.example.yml](deploy.example.yml)。

## 环境变量

见 [.env.example](.env.example)。签发必填：`DP_ID`、`DP_KEY`、`LEGO_EMAIL`、`CERT_DOMAIN`。  
含 `tencent_ssl` / `apigateway` 目标时还需 `TENCENT_SECRET_ID` / `TENCENT_SECRET_KEY`。

## 目录结构

```
cmd/tencent-cert-auto/
internal/cert/       # lego 签发 / 续期
internal/deploy/     # 统一部署（ssl / apigw / local / ssh）
internal/notify/     # 消息推送
internal/app/        # 调度
deploy.example.yml   # 部署目标示例
```
