# Sticker Bot by libost
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/libost/sticker_go)
![GitHub Release](https://img.shields.io/github/v/release/libost/sticker_go)
![GitHub License](https://img.shields.io/github/license/libost/sticker_go)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/libost/sticker_go/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/libost/sticker_go)](https://goreportcard.com/report/github.com/libost/sticker_go)  

[English](./README.md) | [中文](./README_zh.md)

一个基于 Go 语言开发的 Telegram 贴纸提取和转换 Bot，支持单张贴纸和贴纸包的提取，并且能够将 TGS 格式的贴纸转换为 GIF 格式。  
示例机器人：https://t.me/downloaderstickerbot
## 前置条件
- Go 1.25 或更高版本 （二进制文件不需要）
- Docker（仅当启用 TGS 支持时需要）
- Docker 镜像 [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif)（仅当启用 TGS 支持时需要）
- FFmpeg
## 使用方式
1. 克隆仓库并进入目录
```bash
git clone https://github.com/libost/sticker_go.git
cd sticker_go
```
2. 创建配置文件
```bash
cp env.config.yaml config.yaml
```
3. 编辑 `config.yaml` 文件，填入你的 Telegram Bot Token 和其他配置项
4. 运行 Bot
```bash
go run main.go
```

或者，直接从Release页面下载编译好的二进制文件，解压后运行即可。  
不要忘记把 `config.yaml` 文件放在同一目录下。

### Docker 容器运行
也可以直接使用 Docker 运行：
```bash
docker compose up -d --build
```

容器运行时的数据会保存在 `./data/` 目录下，包括 `config.yaml`、`sticker_go.db`、`cache/` 和 `logs/`。第一次启动后，编辑 `./data/config.yaml`，需要的话再重启容器。

如果你在 `config.yaml` 中启用了 TGS 支持（`general.tgs_support: true`），请取消注释 `docker-compose.yml` 里的 Docker socket 挂载，这样容器才能访问宿主机 Docker 守护进程，同时要确保 `edasriyan/lottie-to-gif` 镜像已经在该守护进程中可用。

如果使用 Webhook 模式，请确保 `docker-compose.yml` 中的端口映射和 `config.yaml` 里的 webhook 端口一致。

启动后，在Telegram中输入 `/setadmin <config.yaml中设置的管理员密钥>` 来设置管理员权限，不要泄露管理员密钥给其他人。  
使用命令 `/admin` 来查看管理员功能列表。
## 功能
- 提取 Telegram 消息中的贴纸并转换为 PNG/GIF 格式
- 支持单张贴纸和贴纸包的提取
- 支持在群组中使用， 回复一条贴纸信息并使用命令 `/get` 来提取贴纸
- 支持 TGS 格式的贴纸（需要 Docker 和 [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif) 镜像）
- 支持Webhook模式和轮询模式
## 部分功能说明
### Webhook 模式
如果你希望使用 Webhook 模式，请确保你的服务器能够接受外部请求（i.e., 拥有公网 IP 和可以从外部访问的 443 端口），并在 `config.yaml` 中正确配置 `webhook` 字段。启用 Webhook 后，Bot 将通过 Webhook 接收更新，而不是轮询 Telegram 服务器。  
`nginx_enabled` 字段用于配置是否启用 Nginx 反向代理，如果启用，请确保 Nginx 已正确配置以转发请求到 Bot。  

> 注意：Telegram 要求 Webhook URL 必须使用 HTTPS 协议，并且证书必须是由受信任的 CA 颁发的，或者是自签名证书（需要在配置中开启 `cert.self-signed`）。如果你使用 Nginx 反向代理，建议在 Nginx 中配置 SSL 证书，这样 Bot 就不需要处理 HTTPS 了，Nginx 会将请求转发到 Bot 的 HTTP 端口。  

Nginx 反向代理配置示例：
```nginx
server {
    listen 80;
    server_name example.com;

    # 自动将 HTTP 重定向至 HTTPS
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    # SSL 证书配置 (建议使用 Let's Encrypt / Certbot)
    ssl_certificate /root/.acme.sh/example.com_ecc/fullchain.cer;
    ssl_certificate_key /root/.acme.sh/example.com_ecc/example.com.key;

    # 安全优化
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;


    location /webhook {
        # 限制只允许 Telegram 的 IP 段访问 (可选，增加安全性)
        # allow 149.154.160.0/20;
        # allow 91.108.4.0/22;
        # deny all;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 转发到程序监听的端口 (位于 config.yaml 中的 webhook_port，默认为 8080)
        proxy_pass http://127.0.0.1:8080;

        # Webhook 请求通常很小，禁用缓冲可以提高实时性
        proxy_buffering off;
    }

    # 其他请求直接返回 403 隐藏机器人存在
    location / {
        return 403;
    }
}
```
如果使用以上的配置，那么在 `config.yaml` 中的 `webhook` 字段应该设置为 `https://example.com/webhook`，并且 `nginx_enabled` 设置为 `true`。 
`config.yaml` 配置示例：
```yaml
webhook:
    enabled: true
    nginx_enabled: true
    url: "https://example.com/webhook"
    port: 8080
    secret: "your_webhook_secret" # 可选，设置后会在 Telegram 发送的请求中验证 X-Telegram-Bot-Api-Secret-Token 头部
```
如果你不使用 Nginx，且 Webhook HTTPS 证书是自签名证书，需要在配置中开启 `cert.self-signed`，程序会在调用 `setWebhook` 时自动上传证书给 Telegram：
```yaml
webhook:
  enabled: true
  nginx_enabled: false
  url: "https://example.com/webhook"
  port: 8443
  cert:
    self-signed: true
    cert_path: "./cert.pem"
    key_path: "./key.pem"
  secret: "your_webhook_secret"
```
经验证兼容CDN，测试时使用Cloudflare的CDN。
### TGS 支持
如果你希望支持 TGS 格式的贴纸，请确保你的服务器上安装了 Docker，并且拉取了 [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif) 镜像。启用 TGS 支持后，Bot 将能够将 TGS 格式的贴纸转换为 GIF 格式。  
如果关闭 TGS 支持，Bot 将无法处理 TGS 格式的贴纸，并且相关的贴纸将依照原样返回（.tgs格式）。  

> 警告：启用 TGS 支持会增加系统资源的使用，尤其是在处理大量贴纸时，请确保你的服务器有足够的资源来运行 Docker 和转换过程。
### 贴纸包分卷
如果一个贴纸包包含的贴纸的总大小超过了 Telegram 的限制（通常是 50 MB），Bot 将自动将贴纸包分割成多个 ZIP 文件，每个文件的大小不超过限制。 这确保了用户能够成功下载和使用贴纸包，而不会遇到 Telegram 的文件大小限制问题。  

> 注意：在设计时考虑到了中国大陆服务器的网络上行带宽（通常是 3-5 Mbps），因此发送超时被设置为 3 分钟，以确保大文件能够成功上传，如果频繁遇到超时问题，可以考虑增加服务器的上行带宽或者调整发送超时设置（需重编译）。
### 后台运行
建议在生产环境中使用 `nohup` 或者 `screen` 等工具来后台运行 Bot，以确保它在关闭终端后仍然能够继续运行。 例如：
```bash
screen -S sticker_bot
./sticker_go_linux
```
使用组合键 `Ctrl + A` 然后 `D` 来分离屏幕会话，Bot 将继续在后台运行。 你可以使用 `screen -r sticker_bot` 来重新连接到会话。  

Linux环境下可参考[适用于Linux的systemd服务](/docs/systemd_zh.md)（自v1.8.6起）
### 代理支持
如果你的服务器需要通过代理访问 Telegram API，请在 `config.yaml` 中配置 `proxy` 字段，支持 HTTP 和 SOCKS5 代理。 例如：
```yaml
proxy:
  enabled: true # 务必改成 true 来启用代理
  type: "socks5" # 或 "http"
  host: "127.0.0.1" # 代理服务器地址，不带协议名，可用IP地址或域名
  port: 1080
  username: "proxyuser" # 可选, 无用户名时留空
  password: "proxypass" # 可选，无密码时留空
```
### 捐赠功能
Bot 支持通过 Telegram 的支付功能接受用户的捐赠。 用户可以使用 `/donate` 命令来捐赠 Telegram Stars 给 Bot ，完成支付后，Bot 会记录捐赠信息并发送感谢消息。 用户也可以使用 `/refund` 命令来进行退款。  
如果需要启用捐赠功能，请配置 `config.yaml` 中的 `donation` 字段。  
例如：
```yaml
donation:
  enabled: true # 是否启用捐赠功能，默认是 false，启用后会在命令中添加一个 /donate 命令，用户可以通过该命令捐赠 Telegram Stars 给 Bot
  bonus_enabled: true # 是否启用捐赠奖励功能，默认是 false，启用后用户每次捐赠后会获得1.5倍的使用限制奖励，禁用后用户将不会获得额外的使用限制奖励
  title: "支持开发" # 捐赠信息标题
  description: "如果你喜欢这个项目，欢迎通过以下方式支持开发！" # 捐赠信息描述
  amount_restrict: 
    min: 1 # 最小捐赠金额，单位为 Telegram Star
    max: 10000 # 最大捐赠金额，单位为 Telegram Star
  # 受限于 Apple 和 Google 的政策，不涉及实物的交易不允许使用法币支付，因此暂时只接受 Telegram Star 捐赠
```
### 管理员功能
管理员可以使用 `/admin` 命令来查看管理员功能列表，包括但不限于：
- `/getstats`：查看 Bot 的使用统计信息。
- `/reset`：重置当前用户的使用统计信息。
- `/upgrade`：检查并应用 Bot 的更新（自版本v1.8.1起）。
- `/restart`：重启 Bot。
- `/shutdown`：关闭 Bot。  

管理员功能需要在 `config.yaml` 中设置管理员密钥，并且只有使用正确的管理员密钥的用户才能访问这些功能。 请务必保护好管理员密钥，避免泄露给其他人。
## 许可证
本项目采用 MIT 许可证，详情请参阅 [LICENSE](LICENSE) 文件。