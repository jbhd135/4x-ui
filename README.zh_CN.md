# 4x-ui 中文说明

完整的一键安装、管理、升级及注意事项请阅读 [README.md](README.md)。

```bash
curl -fLsS https://raw.githubusercontent.com/jbhd135/4x-ui/main/install.sh -o /tmp/4x-ui-install.sh && bash /tmp/4x-ui-install.sh
```

当前 `v4.0.5` 支持 Linux x86_64 的 Debian 12+ / Ubuntu 22.04+。新安装默认关闭“客户端自动禁用后重启 Xray”；已有安装保存的设置保持不变，若此前开启，升级后仍需手动关闭并保存。正常限额处理禁止对应客户的新连接，已有长连接可能继续，API 失败等恢复逻辑仍可能重启。保留客户识别码、`10G / 20G / 30G / 无限` 日限额和此前修复。安装包不含任何生产客户或密钥。
