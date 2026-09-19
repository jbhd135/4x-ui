# 4x-ui

gg 现用面板的独立发行项目。保留当前功能，提供新 VPS 一键安装；不会覆盖原来的 GitHub 项目。

当前版本 `v4.0.1` 修复了客户端日限额设为无限后又被周期流量统计写回 10G 的问题。来源和构建说明见 [PROVENANCE.md](PROVENANCE.md)。

## 一键安装

支持 **Linux x86_64（amd64）、Debian 12+ / Ubuntu 22.04+、systemd**。推荐全新 Ubuntu 24.04 VPS；暂不提供 ARM 安装包。

先使用 `sudo -i` 进入 root，然后执行：

```bash
curl -fLsS https://raw.githubusercontent.com/jbhd135/4x-ui/main/install.sh -o /tmp/4x-ui-install.sh && bash /tmp/4x-ui-install.sh
```

安装时可以填写用户名、密码、面板端口和访问路径，回车使用随机值。安装完成会显示登录地址和账号。已有 x-ui 的服务器会执行升级，不会额外安装第二个面板。

无人值守安装（自动生成账号、密码、端口和路径）：

```bash
bash /tmp/4x-ui-install.sh --yes --no-config-prompt
```

指定版本或登录地址参数：

```bash
bash /tmp/4x-ui-install.sh --version v4.0.1 --port 7878 --web-base-path /panel/ --public-host panel.example.com
```

## 保留的功能

- 共享 Reality 入站与独立客户端，设备数、日限额、到期管理。
- 自定义名称、MMDD / YYYYMMDD 日期及 1、3、6、12 个月快捷选项。
- 节点名称包含到期日期及客户后缀，便于检索客户。
- 普通订阅、Clash 订阅及二维码，移动端客户信息。
- 客户端 SOCKS5 静态出口开关，以及当前 gg 版本已有的修复。

## 新服务器须知

- 安装包不带任何生产客户、入站、证书、登录密码或云盘密钥。首次安装是空面板，需自行创建入站或导入自己的数据库。
- 默认使用 HTTP 初始化。正式登录管理前，在面板设置配置域名及有效 HTTPS 证书，或先通过 SSH 隧道访问；同时配置 VPS 防火墙放行所需端口。
- 服务默认时区为 `Asia/Shanghai`，不修改系统时区。日限额默认按北京时间 00:00 刷新；已有数据库和环境文件中的自定义时区保留。
- WARP、SOCKS5 出口账号及运维中心 Google Drive 自动备份不包含在软件发行包中，需在新机器单独配置。
- 不保证网络可达性、解锁效果或防封锁。服务端与客户端仍需正确配置。

## 管理与升级

```bash
x-ui status
x-ui restart
x-ui log
x-ui setting -show
x-ui update
```

兼容原路径：程序 `/usr/local/x-ui`，数据库 `/etc/x-ui/x-ui.db`，服务 `x-ui`。升级前备份到 `/root/4x-ui-backup-*`，包括停服后的数据库目录和原程序；启动失败会尝试自动回滚。备份含敏感数据，请只由 root 保管。

升级默认保留账号、客户和设置。需要修改登录设置时才使用 `--configure`。`--skip-start` 会初始化登录设置但不启动服务。离线安装需要同一 Release 的 `x-ui-linux-amd64.tar.gz` 和 `SHA256SUMS`：

```bash
bash scripts/install-selfhosted.sh --offline-dir /root/4x-ui-release --yes
```

## 源码与许可

基于 [3x-ui](https://github.com/MHSanaei/3x-ui) 和 GG Panel，遵循 [GPL-3.0](LICENSE)。保留 Go module 和服务名以兼容现有数据库。

源码测试：`go test ./...`。GitHub Actions 会测试并编译源码，版本标签会生成带 SHA256 校验的 Linux amd64 发布包。其他语言旧文档及 Docker 文件仅保留作上游开发参考，以本页的一键安装说明为准。
