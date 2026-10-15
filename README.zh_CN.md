# 4x-ui 中文说明

完整的一键安装、管理、升级及注意事项请阅读 [README.md](README.md)。

```bash
curl -fLsS https://raw.githubusercontent.com/jbhd135/4x-ui/main/install.sh -o /tmp/4x-ui-install.sh && bash /tmp/4x-ui-install.sh
```

当前 `v4.0.7` 支持 Linux x86_64 的 Debian 12+ / Ubuntu 22.04+。新增 `编号|家宽| 日期` / `编号|高速| 日期` 命名，快捷创建可开启家宽（绑定现有 SOCKS5 静态出口），设备限制支持 `1 / 2 / 4 / 无限 / 自定义`。自定义名称保留，已有客户的 UUID、到期时间及出口开关不变。

一键安装默认下载最新正式 Release 的安装包，源码更新本身不会更新安装包。保留快捷创建提速、客户识别码、`10G / 20G / 30G / 无限` 日限额和此前修复。新安装仍默认关闭“客户端自动禁用后重启 Xray”；已有安装保存的设置保持不变。正常限额处理禁止对应客户的新连接，已有长连接可能继续，API 失败等恢复逻辑仍可能重启。安装包不含任何生产客户或密钥。
