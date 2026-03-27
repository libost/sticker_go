# systemd 服务配置
## 使用systemd
- 下载二进制可执行[文件](https://github.com/libost/sticker_go/releases)
- 将可执行文件移动到 `/usr/bin/` 目录下，并重命名为 `sticker_go_linux`：
```bash
sudo mv sticker_go_linux /usr/bin/sticker_go_linux
```
- 移动配置文件到 `/etc/sticker_go/` 目录下：
```bash
sudo mkdir -p /etc/sticker_go
sudo mv config.yaml /etc/sticker_go/config.yaml
```
- 创建 systemd 服务文件 `/etc/systemd/system/sticker_go.service`，内容如下：
```ini
[Unit]
Description=Sticker_go, A Simple but Powerful Telegram Sticker Conversion Bot.
After=network.target

[Service]
ExecStart=/usr/bin/sticker_go_linux -d /etc/sticker_go/
Restart=always
RestartSec=1

[Install]
WantedBy=multi-user.target
```
- 重新加载 systemd 配置并启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl start sticker_go
```
- 设置开机自启：
```bash
sudo systemctl enable sticker_go
```
- 查看服务状态：
```bash
sudo systemctl status sticker_go
```
- 查看日志输出：
```bash
sudo journalctl -u sticker_go -f
```