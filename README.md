# Sticker Bot by libost
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

或者，直接从Release页面下载编译好的二进制文件，解压后运行即可。  不要忘记把 `config.yaml` 文件放在同一目录下。

启动后，在Telegram中输入 `/setadmin <config.yaml中设置的管理员密钥>` 来设置管理员权限。
## 功能
- 提取 Telegram 消息中的贴纸并转换为 PNG/GIF 格式
- 支持单张贴纸和贴纸包的提取
## 许可证
本项目采用 MIT 许可证，详情请参阅 [LICENSE](LICENSE) 文件。