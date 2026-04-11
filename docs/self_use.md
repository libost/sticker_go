# Self-use Mode
Self-use mode is a feature designed for users who want to use the bot for personal use without sharing it in group chats. When self-use mode is enabled, the bot will disable certain features that are not suitable for personal use and will not set commands in group chats. This mode is ideal for users who want to use the bot privately without exposing it to a wider audience.
## Enabling Self-use Mode
To enable self-use mode, you need to set the `self_use` field to `true` in the `misc` section of the `config.yaml` file. Here is an example of how to enable self-use mode:
```yaml
misc:
  self_use: true
```
WARNING: You need to type your user ID to the `owner_id` field in the `config.yaml` file to use self-use mode, otherwise you won't be able to access the bot after enabling this mode. For example:
```yaml
misc:
  self_use: true
  owner_id: 123456789
```
## Effects of Self-use Mode
When self-use mode is enabled, the following changes will occur:
- The bot will not set commands in group chats, so users won't see the bot's commands when they interact with it in a group chat.
- Certain features that are designed for group use may be disabled or modified to better suit personal use. For example, the `/usage` command will display a message indicating that there are no usage limits in self-use mode.
- The `/about` command will include a message indicating that self-use mode is enabled.
- The donation feature will be forcibly disabled as it is not suitable for self-use mode.