# Sticker Bot by libost
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/libost/sticker_go)
![GitHub Release](https://img.shields.io/github/v/release/libost/sticker_go)
![GitHub License](https://img.shields.io/github/license/libost/sticker_go)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/libost/sticker_go/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/libost/sticker_go)](https://goreportcard.com/report/github.com/libost/sticker_go)  

[中文](./README_zh.md) | [English](./README.md)  

A Telegram sticker extraction and conversion Bot developed in Go, supporting both single stickers and sticker packs, with the ability to convert TGS format stickers to GIF format.  
Template: https://t.me/downloaderstickerbot
## Prerequisites
- Go 1.25 or higher (not required for binary releases)
- Docker (only required if TGS support is enabled)
- Docker image [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif) (only required if TGS support is enabled)
- FFmpeg
## Usage
1. Clone the repository and enter the directory
```bash
git clone https://github.com/libost/sticker_go.git
cd sticker_go
```
2. Create configuration file
```bash
cp env.config.yaml config.yaml
```
3. Edit the `config.yaml` file and fill in your Telegram Bot Token and other configuration options
4. Run the Bot
```bash
go run main.go
```

or, download the pre-compiled binary from the Release page, extract it, and run it.  
Don't forget to place the `config.yaml` file in the same directory.

### Docker Container
You can also run the bot inside Docker:
```bash
docker compose up -d --build
```

The container stores its runtime data in `./data/`, including `config.yaml`, `sticker_go.db`, `cache/`, and `logs/`. After the first start, edit `./data/config.yaml` and restart the container if needed.

If you enable TGS support in `config.yaml` (`general.tgs_support: true`), uncomment the Docker socket mount in `docker-compose.yml` so the container can access the host Docker daemon, and make sure the `edasriyan/lottie-to-gif` image is available on that daemon.

If you use webhook mode, make sure the port mapping in `.env.example` matches the webhook port in `config.yaml`.

After starting, input `/setadmin <admin key set in config.yaml>` in Telegram to set admin privileges, and do not leak the admin key to others.  
Use the command `/admin` to view the list of admin features.
## Features
- Extract Telegram messages' stickers and convert them to PNG/GIF format
- Support for extracting single stickers and sticker packs
- Support for use in groups, reply to a sticker message and use the command `/get` to extract the sticker
- Support for TGS format stickers (requires Docker and [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif) image)
- Support for Webhook mode and polling mode
## Partial Feature Explanation
### Webhook Mode
If you want to use Webhook mode, ensure your server can accept external requests (i.e., has a public IP and a 443 port accessible from the outside), and configure the `webhook` field in `config.yaml` correctly. After enabling Webhook, the Bot will receive updates via Webhook instead of polling the Telegram server.  
The `nginx_enabled` field is used to configure whether to enable Nginx reverse proxy. If enabled, ensure Nginx is correctly configured to forward requests to the Bot.  

> Note: Telegram requires Webhook URLs to use HTTPS protocol, and the certificate must be issued by a trusted CA or be a self-signed certificate (requires enabling `cert.self-signed` in the configuration). If you are using Nginx reverse proxy, it is recommended to configure an SSL certificate in Nginx, so that the Bot does not need to handle HTTPS, and Nginx will forward requests to the Bot's HTTP port.

Nginx Reverse Proxy Configuration Example:
```nginx
server {
    listen 80;
    server_name example.com;

    # Automatically redirect HTTP to HTTPS
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    # SSL Certificate Configuration (Let's Encrypt / Certbot is recommended)
    ssl_certificate /root/.acme.sh/example.com_ecc/fullchain.cer;
    ssl_certificate_key /root/.acme.sh/example.com_ecc/example.com.key;

    # Security Optimization
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;


    location /webhook {
        # allow only Telegram IP ranges (optional, can be commented out for testing, but recommended for production)
        # allow 149.154.160.0/20;
        # allow 91.108.4.0/22;
        # deny all;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Forward to the port the program is listening on (configured in config.yaml's webhook_port, default is 8080)
        proxy_pass http://127.0.0.1:8080;

        # Webhook requests are usually small, disabling buffering can improve real-time performance
        proxy_buffering off;
    }

    # Other requests return 403 to hide the bot's presence
    location / {
        return 403;
    }
}
```
If you use the above configuration, then the `webhook` field in `config.yaml` should be set to `https://example.com/webhook`, and `nginx_enabled` should be set to `true`.
`config.yaml` Configuration Example:
```yaml
webhook:
    enabled: true
    nginx_enabled: true
    url: "https://example.com/webhook"
    port: 8080
    secret: "your_webhook_secret" # Optional, if set, the bot will verify the X-Telegram-Bot-Api-Secret-Token header in incoming requests
```
If you do not use Nginx, and the Webhook HTTPS certificate is a self-signed certificate, you need to enable `cert.self-signed` in the configuration. The program will automatically upload the certificate to Telegram when calling `setWebhook`:
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
CDN Compatibility Verified, Tested with Cloudflare's CDN.
### TGS Support
If you want to support TGS format stickers, make sure you have Docker installed on your server and have pulled the [`edasriyan/lottie-to-gif`](https://hub.docker.com/r/edasriyan/lottie-to-gif) image. Enabling TGS support will allow the Bot to convert TGS format stickers to GIF format.  
If you disable TGS support, the Bot will not be able to process TGS format stickers, and related stickers will be returned as-is (in .tgs format).

> Warning: Enabling TGS support will increase system resource usage, especially when processing a large number of stickers. Please ensure your server has sufficient resources to run Docker and the conversion process.
### Sticker Pack Splitting
If a sticker pack contains stickers whose total size exceeds Telegram's limit (typically 50 MB), the Bot will automatically split the sticker pack into multiple ZIP files, each not exceeding the limit. This ensures users can successfully download and use the sticker pack without encountering Telegram's file size limitations.  
### Background Running
It is recommended to use tools like `nohup` or `screen` to run the Bot in the background in production environments, ensuring it continues running after the terminal is closed. For example:
```bash
screen -S sticker_bot
./sticker_go_linux
```
Use the key combination `Ctrl + A` followed by `D` to detach the screen session, and the Bot will continue running in the background. You can reconnect to the session using `screen -r sticker_bot`.

For Linux environments, you can refer to the [systemd service for Linux](/docs/systemd.md) (available from v1.8.6).
### Proxy Support
If your server needs to access the Telegram API through a proxy, please configure the `proxy` field in `config.yaml`. Both HTTP and SOCKS5 proxies are supported. For example:
```yaml
proxy:
  enabled: true # Whether to enable proxy support, default is false, enable it if your server needs to access Telegram API through a proxy
  type: "socks5" # or "http"
  host: "127.0.0.1" # Proxy server address, without protocol, can be an IP address or domain name
  port: 1080
  username: "proxyuser" # Optional, leave empty if no username is required
  password: "proxypass" # Optional, leave empty if no password is required
```
### Donation Support
The Bot supports accepting donations from users through Telegram's payment feature. Users can use the `/donate` command to donate Telegram Stars to the Bot, and after completing the payment, the Bot will record the donation information and send a thank-you message. Users can also use the `/refund` command to request a refund.  
If you want to enable the donation feature, please configure the `donation` field in `config.yaml`. For example:
```yaml
donation:
  enabled: true # Whether to enable donation functionality, default is false, enable it if you want to accept donations
  bonus_enabled: true # Whether to enable donation bonus functionality, default is false, enable it if you want users to receive bonus usage limits after donating
  title: "Support Development" # Donation information title
  description: "If you like this project, please consider supporting development through the following ways!" # Donation information description
  amount_restrict: 
    min: 1 # Minimum donation amount in Telegram Stars
    max: 10000 # Maximum donation amount in Telegram Stars
  # Due to Apple and Google's policies, transactions that do not involve physical goods are not allowed to use fiat currency payments, so currently only Telegram Star donations are accepted.
```
### Administrator Features
Administrators can use the `/admin` command to view the list of administrator features, including but not limited to:
- `/getstats`：View Bot usage statistics.
- `/reset`：Reset the current user's usage statistics.
- `/upgrade`：Check and apply Bot updates (starting from version v1.8.1).
- `/restart`：Restart the Bot.
- `/shutdown`：Shutdown the Bot.  

Administrator features require setting an administrator key in `config.yaml`, and only users with the correct administrator key can access these features. Please ensure to protect the administrator key and avoid leaking it to others.
## License
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.