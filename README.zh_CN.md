# 4x-ui 中文说明

完整的一键安装、管理、升级及注意事项请阅读 [README.md](README.md)。

```bash
curl -fLsS https://raw.githubusercontent.com/jbhd135/4x-ui/main/install.sh -o /tmp/4x-ui-install.sh && bash /tmp/4x-ui-install.sh
```

当前 `v4.0.8` 支持 Linux x86_64 的 Debian 12+ / Ubuntu 22.04+。客户设备限制新增 90 秒切网宽限，仅限制超限账号及来源的新连接，不封整个公网 IP；恢复后自动解除。保留编号命名、家宽开关和设备规格。已有客户的 UUID、到期时间及出口开关不变。详细规则及限制见 [README.md](README.md#v408-客户设备限制)。

一键安装默认下载最新正式 Release 的安装包，源码更新本身不会更新安装包。保留快捷创建提速、客户识别码、`10G / 20G / 30G / 无限` 日限额和此前修复。新安装仍默认关闭“客户端自动禁用后重启 Xray”；已有安装保存的设置保持不变。正常限额处理禁止对应客户的新连接，已有长连接可能继续，API 失败等恢复逻辑仍可能重启。安装包不含任何生产客户或密钥。
