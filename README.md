# NativeJet

macOS 原生 Mihomo/Clash.Meta 管理器。使用 MyGo Native UI，界面没有 WebView。
安装包仅包含 App 与 helper，不附带、不链接代理内核。

## 功能

- 配置：订阅地址、添加 YAML 文件、粘贴内容。本地文件导入为副本，可在软件内编辑和保存。
- 订阅原文只读，支持定时更新、ETag 和流量信息保存；更新只保存新原文，不自动应用。
- 全局覆盖与配置专属覆盖；最终 YAML、字段来源和原始行号。
- 内核：打开目录、自行放置后扫描、架构和版本识别、官方 GitHub 下载、SHA-256 身份记录与版本选择。
- 内核生命周期和 TUN。内核运行时，应用配置会用新配置重启内核。启动失败时尝试恢复此前版本。
- 连接查看与关闭、策略组选择；实时内核日志、标准输出和错误输出，支持来源/级别筛选、搜索、完整内容复制与导出。
- 内核启动失败时显示本次进程报错。内核运行时产生的错误也可在日志页查看。
- 配置历史、恢复、已应用配置与当前预览的原生差异查看器。

首版面向 macOS 13+，原生构建本机架构。Windows 属于后续阶段。

## 构建与运行

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

## 在界面中安装、授权与测试 TUN

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

## 配置语义

固定优先级：

```text
原始配置 → 全局覆盖 → 当前配置覆盖 → 最终 YAML
```

对象递归合并；列表整体替换；标量替换；显式 `null` 删除字段，`false` 保持布尔语义。
未知内核字段保留。YAML 锚点和合并键可用；重复键、多文档、循环锚点和超限内容会被拒绝。
首版不执行配置转换脚本。

初始全局覆盖明确包含 TUN、端口和日志级别，用户可直接查看、编辑。

Helper 按最终 YAML 中的控制接口配置连接 Mihomo，依次采用配置中的 Unix、HTTP 或 HTTPS 地址。
HTTP 和 HTTPS 请求使用配置中的 `secret`。控制地址、密钥和外部面板字段均按同样的覆盖规则合并。
连接、策略组和实时 API 日志依赖控制接口。进程输出由 helper 直接采集。
实际配置文件保留完整内容，预览与差异查看对常见敏感值脱敏；显示的哈希对应实际提交文件。

“应用配置”把保存后的配置与覆盖合成为最终 YAML。内核已停止时，生成的版本供下次启动使用。
内核运行时，helper 停止旧进程，再用新 YAML 启动 Mihomo。由 Mihomo 在启动时解析配置。
保存和订阅更新之后，需要点击“应用配置”才会切换版本。

## 目录与权限

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
便于保留 Go 业务协议并在 Windows 阶段替换传输层。Mihomo 的控制接口独立，由最终 YAML 指定。
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
