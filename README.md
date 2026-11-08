# NativeJet

macOS、Windows 和 Linux 原生 Mihomo/Clash.Meta 管理器。使用 MyGo Native UI，界面没有 WebView。
安装包包含 App 和后台 helper，不附带、不链接代理内核。

## 功能

- 配置：订阅地址、添加 YAML 文件、粘贴内容。本地文件导入为副本，可在软件内编辑和保存。
- 订阅支持 Clash/Mihomo YAML、明文节点列表和 Base64 节点列表。订阅原文只读，支持定时更新、ETag 和流量信息保存，刷新后需手动应用。
- 可命名、切换的多份全局覆盖与配置专属覆盖；最终 YAML、字段来源和原始行号。
- 内核：打开目录、自行放置后扫描、架构和版本识别、官方 GitHub 下载、SHA-256 身份记录与版本选择。
- 内核生命周期和 TUN。内核运行时，应用配置会用新配置重启内核。启动失败时尝试恢复此前版本。
- 连接查看与关闭、策略组选择；实时内核日志、标准输出和错误输出，支持来源/级别筛选、搜索、完整内容复制与导出。
- 内核启动失败时显示本次进程报错。内核运行时产生的错误也可在日志页查看。
- 配置历史、恢复、已应用配置与当前预览的原生差异查看器。

支持 macOS 13+、Windows 10/11 和 Linux（x64、ARM64）。Linux 提供面向 Ubuntu 24.04 等使用 systemd 的 Debian 系发行版的安装包。

## Windows 构建与使用

1. 解压与系统架构匹配的 Windows ZIP，将 `NativeJet.exe` 和 `nativejet-helper.exe` 放在同一目录，再运行 `NativeJet.exe`。关闭窗口后可通过系统托盘恢复。
2. 在「内核」下载官方 Windows ZIP，或将解压后的同架构 Mihomo EXE 放入内核目录后扫描并选用。扫描读取 PE 和 Go 构建信息，无需执行待授权的 EXE。
3. 在「设置」安装后台服务并完成 UAC 授权。切换到未授权的内核时，管理员需要批准该内核的 SHA-256。普通用户服务可用于不启用 TUN 的配置。
4. 导入配置并应用。Windows 的默认覆盖包含 TUN 和 Mihomo 的本地命名管道控制接口，具体值可在覆盖页面编辑。使用从 macOS 带来的 YAML 时，可将 Unix 控制地址换成 Windows 命名管道或带 `secret` 的回环 HTTP。

Windows App 和 helper 使用受 Windows ACL 保护的本地命名管道。系统服务按一个已授权的用户 SID 管理配置，状态和内核运行目录位于 `%ProgramData%\NativeJet`，GUI 偏好位于 `%LOCALAPPDATA%\NativeJet`。卸载后台服务会保留系统配置数据。

需要 Go 1.27.3+。在 macOS 或 Windows Git Bash 中构建：

```sh
bash scripts/build-windows.sh amd64
bash scripts/build-windows.sh arm64
```

输出在 `build/windows-amd64` 和 `build/windows-arm64`。Windows 上可以执行 `go test ./...`、`go vet ./...`，以及 `nativejetctl.exe -installed snapshot` 检查已安装服务。GitHub Actions 为两个架构生成 ZIP。

## Linux 构建与使用

1. 用 `sudo apt install ./NativeJet-linux-amd64.deb` 安装对应架构的包，ARM64 使用 `NativeJet-linux-arm64.deb`。从应用菜单或终端运行 `nativejet`。App 使用 GTK 3。
2. 在「内核」下载官方 Linux 内核，或将具有执行权限的同架构 Mihomo ELF 文件放入内核目录并扫描、选用。
3. 在「设置」点击「安装并授权后台服务」，通过桌面的 polkit 管理员提示。App 在授权前自动启动包内 helper，授权后切换至 `dev.nativejet.helper.service`，并迁移现有配置。内核替换后按提示授权新的 SHA-256。
4. 导入配置并应用，再启动内核。默认覆盖启用 TUN，使用 `external-controller-unix: mihomo.sock`。系统需提供 `/dev/net/tun`。关闭 TUN 后，同一个 App 可直接使用以登录用户权限运行的 helper。

系统服务仅允许授权的本机 UID 连接，配置保存在 `/var/lib/nativejet`，GUI 偏好保存在用户的 XDG 配置目录。通过 App 卸载服务会停用 systemd 单元并保留配置。没有可用托盘的桌面环境会正常显示和关闭窗口。

需要 Go 1.27.3+、`dpkg-deb` 和 OpenSSL。运行以下命令可构建两个架构，GitHub Actions 也会生成同名安装包。

```sh
bash scripts/build-linux.sh amd64
bash scripts/build-linux.sh arm64
```

输出为 `build/NativeJet-linux-amd64.deb` 和 `build/NativeJet-linux-arm64.deb`。系统服务可用 `nativejetctl -installed snapshot` 检查，日志可用 `journalctl -u dev.nativejet.helper.service` 查看。

## macOS 构建与运行

需要 Go 1.27.3+ 和 Xcode Command Line Tools。MyGo 固定为 v0.7.4。
项目使用少量 Objective-C/C 桥接 ServiceManagement、Security 和 libproc；
界面与业务代码为 Go。构建脚本开启 CGO，无需 Bun、Node 或 Swift 包依赖。
另需 Python 3：构建脚本生成 MyGo 的临时模块副本，修正原生 macOS 窗口对
`NoBackgroundThrottling` 的支持。主窗口在后台或显示刷新暂停时仍更新操作状态，
防止操作已经结束却继续显示“正在处理”。补丁校验原源码哈希，不改动依赖缓存。

```sh
make check
make build
open build/NativeJet.app
```

关闭主窗口（红色按钮或 `⌘W`）会隐藏 Dock 图标，NativeJet 继续在 macOS 菜单栏运行。
点击菜单栏的火箭剪影，选择“打开 NativeJet”可恢复原窗口和 Dock 图标，保留编辑中的内容。
选择“退出 NativeJet”或在主窗口按 `⌘Q` 可退出界面；内核仍由独立的后台服务管理。

在「设置 → 启动」可分别开启以下选项，默认均关闭：

- **开机启动**：登录 macOS 后自动打开 NativeJet。登录项由 macOS 管理；若提示待授权，可从设置页打开系统登录项设置。
- **启动时自动启动内核**：下次启动 App 并连接后台服务后，使用所选内核和已应用配置启动。已运行的内核保持运行；更换内核后请重新应用配置，TUN 所需授权也需预先完成。
- **启动时不显示窗口**：下次启动时只在菜单栏显示，点击「打开 NativeJet」可恢复主窗口。打开 Clash 订阅链接时仍会显示导入窗口。

网站的一键导入链接可用 `clash://install-config?url=<编码后的 HTTPS 订阅地址>&name=<编码后的订阅名称>`。
`name` 可省略。NativeJet 会预填订阅，点击“下载并添加订阅”才会下载。启用订阅时另行点击“应用配置”。
如系统把链接交给其他 Clash 客户端，可在“设置”中手动点击“将 NativeJet 设为 Clash 链接默认应用”。

图标的 SVG 源文件位于 `packaging/macos/artwork/`。修改后可用 `python3 scripts/render-icons.py`
重新生成图标（需 `rsvg-convert`）；正常构建直接使用已生成的资源。

未提供 Developer ID 时，构建产物采用 ad-hoc 签名。打开 App 的“设置”，
点击“安装并授权后台服务”，在 macOS 原生弹窗中输入管理员密码即可。
本地构建可使用 TUN，不需要手动启动终端、执行 sudo 或重新导入配置。
未安装服务时，App 自动连接普通用户 helper；所有配置继续可编辑和保存。

可用独立数据目录运行，不影响日常数据：

```sh
build/NativeJet.app/Contents/MacOS/NativeJet \
  --dev --data-dir /private/tmp/nativejet-development
```

点击“内核 → 打开目录”，自行放入解压后的 macOS 可执行文件，再点击“重新扫描”。
内核应有执行权限，且架构与当前机器匹配。扫描的版本探测以登录用户权限运行。
“使用所选内核”会将实际文件内容校验并复制到受管理目录；启动时再次核对哈希。

## 在 macOS 上安装、授权与测试 TUN

1. 在“内核”选择自己放入或从官方来源下载的内核。
2. 打开“设置”，点击“安装并授权后台服务”，完成系统管理员授权。
3. 等待显示“已连接 · 可以启动 TUN”。现有配置、覆盖、内核副本、缓存和历史自动迁移；
   普通用户数据目录完整保留。迁移不会把尚未应用的修改替换成已应用版本。
4. 在“配置 → 覆盖配置 → 全局覆盖”中将 `tun.enable` 设为 `true`，保存覆盖并点击“应用配置”，再启动内核。
   测试时请先在其他代理客户端中停止 TUN，避免路由、DNS 和端口冲突。

取消授权会恢复普通用户服务；安装失败会尝试恢复原服务及状态文件。
服务安装完成后自动连接，关闭窗口仍可继续运行。
重新启动 App 后，会直接连接已安装的后台服务。选择未获授权的内核时，点击“授权所选内核”。

安装器使用 macOS Authorization Services 的管理员弹窗，经系统 `/bin/sh` 启动固定安装脚本。
脚本先将 helper 复制到 root 私有目录并校验 SHA-256，再执行安装；不直接以 root 执行用户可写的程序路径。
此授权入口使用已弃用但当前 macOS SDK 仍提供的 `AuthorizationExecuteWithPrivileges`，
后续系统兼容性需要持续回归。安装后通过 launchd 管理独立 LaunchDaemon，
不是依赖付费 Developer ID 的 SMAppService 注册流程。

服务和公开身份记录位于 `/Library/PrivilegedHelperTools`，LaunchDaemon 位于
`/Library/LaunchDaemons`。App 验证已安装 helper 的身份。已安装服务接受所有本机账号通过 Unix socket 发出的请求，按连接方 UID 使用对应的配置目录。
服务只以 root 执行管理员批准过 SHA-256 的内核。
版本扫描始终使用普通用户权限。更换文件不能沿用旧内核授权。

“卸载后台服务，保留配置”需要先停止内核。卸载后配置原文和覆盖自动复制回普通用户服务；
系统中的完整历史与缓存保留，重新安装可继续使用。不删除用户配置。

正式对外分发还需 Developer ID 签名、公证和真实机器回归：

```sh
NATIVEJET_SIGN_IDENTITY='Developer ID Application: Your Name (YOURTEAMID)' \
NATIVEJET_TEAM_ID='YOURTEAMID' make build
NATIVEJET_NOTARY_PROFILE='your-keychain-profile' bash scripts/notarize.sh
```

### GitHub Actions 构建

推送到 `master`、提交以 `master` 为目标的 PR、推送 `v*` 标签，或在 Actions 中手动运行
「Build NativeJet」，都会构建并检查 Apple Silicon 和 Intel 两种版本。构建成功后可在该次运行的
Artifacts 下载 `NativeJet-macos-arm64.zip` 或 `NativeJet-macos-amd64.zip`，解压后得到包含
后台 helper 的 `NativeJet.app`。CI 产物使用 ad-hoc 签名，供测试使用。

## 配置语义

固定优先级：

```text
原始配置 → 全局覆盖 → 当前配置覆盖 → 最终 YAML
```

对象递归合并；列表整体替换；标量替换；显式 `null` 删除字段，`false` 保持布尔语义。
未知内核字段保留。YAML 锚点和合并键可用；重复键、多文档、循环锚点和超限内容会被拒绝。
首版不执行配置转换脚本。

完整 YAML 和仅含 `proxies:` 的 YAML 都作为订阅原文保存。URI 列表在本地转换为 `proxies:`，
保留列表顺序和已有节点参数。支持常见的 Shadowsocks、ShadowsocksR、VMess、VLESS、Trojan、
Hysteria、Hysteria2、TUIC、AnyTLS、SOCKS5 和 HTTP(S) 节点链接。遇到无法映射的节点或参数时，
报告节点行号并保留上次成功下载的版本。策略组、规则等由用户自己的覆盖配置补充。

在“配置 → 覆盖配置 → 全局覆盖”中可从当前覆盖新建副本、重命名、删除和切换。
旧版的单份全局覆盖会自动保留为“默认”。全新安装的默认覆盖包含 TUN、端口和日志级别。
每次合并只使用选中的一份，切换后点击“应用配置”才会更新已应用版本。
已安装旧版系统后台服务时，可在“设置”中更新后台服务以启用多份覆盖。

Helper 按最终 YAML 中的控制接口配置连接 Mihomo，依次采用配置中的 Unix、HTTP 或 HTTPS 地址。
HTTP 和 HTTPS 请求使用配置中的 `secret`。控制地址、密钥和外部面板字段均按同样的覆盖规则合并。
连接、策略组、实时 API 日志和实时流量依赖控制接口。进程输出由 helper 直接采集。
主窗口侧边栏底部和 macOS 菜单栏共用 helper 从 Mihomo `/traffic` 取得的实时上下行速度。
侧边栏显示最近 5 分钟的速率曲线。历史由 helper 保存在内存，采样中断时曲线留空。
在“设置 → 菜单栏速度”可选择单列（上下排列）、双列（左右并排）或关闭，选择会自动保存。关闭时菜单栏仅显示火箭图标。
未收到数据时显示「—」；内核上报零流量时显示 `0 B/s`。
若生效配置未启用控制接口，可在覆盖配置中添加 `external-controller-unix: mihomo.sock` 并应用。
侧栏的「GeoData」显示运行中的 Mihomo 报告的下载源，以及运行目录内数据库的文件名、大小和修改时间。
「立即更新」由 helper 转发给 Mihomo，需要内核运行且控制接口可用。GeoData 由 Mihomo 按需下载，发行包不内置数据库。
实际配置文件保留完整内容，预览与差异查看对常见敏感值脱敏；显示的哈希对应实际提交文件。

“应用配置”把保存后的配置与覆盖合成为最终 YAML。内核已停止时，生成的版本供下次启动使用。
内核运行时，helper 停止旧进程，再用新 YAML 启动 Mihomo。由 Mihomo 在启动时解析配置。
保存和订阅更新之后，需要点击“应用配置”才会切换版本。

## macOS 目录与权限

用户放置内核的目录：

```text
~/Library/Application Support/NativeJet/kernels
```

生产 helper 数据（按登录 UID 隔离，目录 0700、配置 0600）：

```text
/Library/Application Support/NativeJet/users/<uid>/
├── state.json                 配置原文、覆盖和历史元数据
├── cores/<sha256>/mihomo      受管理的内核副本
└── runtime/
    └── revisions/<id>.yaml    实际交给内核的完整配置
```

对于较长的数据路径，runtime 沿用已有的私有临时目录。
开发数据保存在 `--data-dir` 下的 `development`，与生产配置分离。
配置可能包含订阅凭据和节点密码，文件依靠操作系统权限保护，并非加密数据库。

App 只调用版本化 helper 协议，不直接调用 core API。当前传输采用 Unix socket，
Windows 版沿用同一 Go 业务协议并通过命名管道传输。Mihomo 的控制接口独立，由最终 YAML 指定。
helper 不提供通用 shell 命令或任意 core API 转发接口。

## 开发 CLI 与测试

`build/nativejetctl` 用于普通用户开发 helper。

```sh
build/nativejetctl -socket /private/tmp/nativejet-development/helper.sock snapshot
build/nativejetctl -socket /private/tmp/nativejet-development/helper.sock \
  -params '{"id":"PROFILE_ID"}' config.preview
```

`make check` 运行 race detector 与 `go vet`。进程集成测试使用测试可执行文件作为受控
core，仅在需要时创建本地控制接口，不建立 TUN、代理监听端口或修改 DNS/路由。测试包括：

- 覆盖优先级、列表替换、false/null、未知字段与来源追踪；
- YAML 锚点、无效输入与脱敏；
- 私有持久化、损坏数据保留、符号链接拒绝；
- 订阅原文只读、失败更新保留旧内容；
- 真实子进程启动、应用时重启、启动失败时恢复旧版本；
- App/helper 协议、非法操作拒绝、本地配置保存；
- 配置与历史迁移、未应用修改保留、文件篡改和路径穿越拒绝、内核授权拦截。

管理员弹窗、服务安装和真实 TUN 转发需要在真实机器上另行验证。
测试通过不等于已经完成这部分系统回归。

具体已验证路径和限制见 [本机验证记录](docs/VALIDATION.md)。
