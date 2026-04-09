# systemd Service for Linux
[中文](./systemd_zh.md) | [English](./systemd.md)
## Installation and Usage
- Download the binary executable [file](https://github.com/libost/sticker_go/releases)
- Move the executable file to the `/usr/bin/` directory and rename it to `sticker_go_linux`:
```bash
sudo mv sticker_go_linux /usr/bin/sticker_go_linux
```
- Move the configuration file to the `/etc/sticker_go/` directory:
```bash
sudo mkdir -p /etc/sticker_go
sudo mv config.yaml /etc/sticker_go/config.yaml
```
- Create the systemd service file `/etc/systemd/system/sticker_go.service` with the following content:
```ini
[Unit]
Description=Sticker_go, A Simple but Powerful Telegram Sticker Conversion Bot.
After=network.target

[Service]
ExecStart=/usr/bin/sticker_go_linux -d /etc/sticker_go/
# Supports hot reload of configuration since v1.9.3, send SIGHUP signal to reload configuration
# Warning: Hot reload does not apply to changes in Telegram Bot Token, you must restart the service for changes to take effect
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=1

[Install]
WantedBy=multi-user.target
```
- Reload the systemd configuration and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl start sticker_go
```
- Enable the service to start on boot:
```bash
sudo systemctl enable sticker_go
```
- Check the service status:
```bash
sudo systemctl status sticker_go
```
- View the service logs:
```bash
sudo journalctl -u sticker_go -f
```
- Stop the service:
```bash
sudo systemctl stop sticker_go
```
- Restart the service:
```bash
sudo systemctl restart sticker_go
```
## Remove the program and all its components
- Stop the service:
```bash
sudo systemctl stop sticker_go
```
- Disable the service:
```bash
sudo systemctl disable sticker_go
```
- Delete the service file:
```bash
sudo rm /etc/systemd/system/sticker_go.service
```
- Delete the executable file:
```bash
sudo rm /usr/bin/sticker_go_linux
```
- Delete the configuration file:
```bash
sudo rm -r /etc/sticker_go
```
- Reload the systemd configuration:
```bash
sudo systemctl daemon-reload
```