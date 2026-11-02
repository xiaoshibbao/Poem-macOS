# Poem Launcher · macOS 版

Poem Launcher 的 macOS 移植版。它是一个《崩坏：星穹铁道》启动器，支持私服、官服和 B服三种登录渠道，提供战斗时停、帧率上限、解除多开、移动端 UI、角色去虚化、游戏语言切换和本地 Lua 脚本等功能。

macOS 上没有原生的 Windows 版游戏，所以游戏通过 **Wine**（CrossOver、Whisky、Game Porting Toolkit、Homebrew 等）运行。Poem 负责在同一个 Wine 容器里启动游戏、加载功能模块并提供本地代理。

技术栈：桌面壳 [Wails v3](https://v3.wails.io/)，后端 Go，界面 React + TypeScript + Vite。界面与 Windows 版一致，另外新增了「运行环境」面板。

## 工作方式

```
┌──────────── macOS ────────────┐      ┌──────────── Wine 容器 ────────────┐
│ Poem.app                      │      │ poem-helper.exe                   │
│  · 界面、设置                  │ wine │  · 创建游戏进程（挂起）            │
│  · 本地 HTTP/HTTPS 代理 ───────┼──────┼─▶ · 按阶段注入功能模块             │
│  · 语言、渠道、Lua 打包        │ JSON │  · 证书写入容器的受信任根证书存储   │
│  · 解析助手输出的 JSON 事件    │◀─────┤  · 去虚化共享配置、等待游戏退出    │
└───────────────────────────────┘      └───────────────────────────────────┘
```

- `poem-helper.exe` 是本仓库编译出的 Windows 小程序，内置在 Poem.app 中。Poem 通过 `wine poem-helper.exe launch <计划文件>` 调用它。助手与游戏运行在同一个 wineserver 中，所以可以创建游戏进程并注入模块。
- 代理运行在 macOS 一侧。Wine 与 macOS 共用网络，游戏可以直接访问 `127.0.0.1` 上的代理端口。
- macOS 路径会自动换算成 Wine 内的路径：`drive_c` 里的文件对应 `C:\`，其他位置对应 `Z:\`。

## 功能

| 分类 | 内容 |
| --- | --- |
| 运行环境 | 自动检测 CrossOver / Whisky / GPTK / Homebrew 的 Wine 和已有容器，也可手动选择；支持额外环境变量（如 `WINEMSYNC=1`）。 |
| 登录渠道 | 私服 / 官服 / B服。官服和 B服共用同一个正式国服客户端目录，启动时切换 `config.ini` 的渠道字段；B服首次使用需下载官方渠道组件（约 85 MB）。 |
| 游戏功能 | 战斗时停、帧率上限、解除多开、移动端 UI、角色去虚化，下次启动游戏时生效。 |
| 网络代理 | 仅私服。把游戏流量转发到本地服务器、预设服务器或自定义地址；可选 RSA 公钥替换和游戏内网页重定向。 |
| 游戏语言 | 分别设置文字和语音语言；首次修改前自动备份，可一键恢复。 |
| Lua 脚本 | 添加本地 UTF-8 `.lua` 文件，游戏进入可游玩场景后按顺序自动执行，也可对运行中的游戏手动重新执行。 |

## 使用

1. 从 [Releases](../../releases) 或 Actions 构建产物下载 `Poem-*-macOS.zip`，解压后把 `Poem.app` 拖进「应用程序」。
   应用只做了 ad-hoc 签名，没有经过公证。首次打开时请右键点击 →「打开」，或运行：
   ```bash
   xattr -dr com.apple.quarantine /Applications/Poem.app
   ```
2. 先确认游戏本身已经能在你的 Wine 容器里运行。
3. 打开 Poem，在「运行环境」里选择 Wine 程序和容器（prefix）。
4. 点击「打开模块文件夹」，放入需要的预编译模块（见下文）。
5. 点击启动栏左侧的文件夹按钮，选择容器中的 `StarRail.exe`，再选择登录渠道并调整各项设置。
6. 点击「启动游戏」。

有游戏运行时，关闭窗口或按 ⌘Q 只会最小化 Poem，以保持代理连接。

### 预编译模块

以下模块不随本仓库分发，需要自行放入 `~/Library/Application Support/Poem/modules/`。缺少某个模块只影响对应的功能开关。

| 文件 | 功能 |
| --- | --- |
| `ProxyHook.dll` | 私服网络代理 |
| `GameFeatures.dll` | 战斗时停、帧率上限、解除多开、移动端 UI |
| `VisualPatch.dll` | 角色去虚化 |

`LuaEntry.dll`（本地 Lua 桥接）和 `poem-helper.exe` 由本仓库源码编译，已内置。

### 本地文件

- 设置、代理证书、渠道备份、语言备份和 `launch.log` 位于 `~/Library/Application Support/Poem/`。
- 每次启动的运行时模块释放在系统临时目录 `Poem-runtime-*`，游戏退出后清理。
- 启用代理时，Poem Local Proxy 根证书只写入所选 Wine 容器的受信任根证书存储，不会改动 macOS 钥匙串。

## 与 Windows 版的差异

- 游戏通过 Wine 启动，需要先配置运行环境。
- 不包含 Windows 专属的诊断与网络功能：WER 崩溃转储、线程跟踪、控制台记录、VPN 直连路由和系统代理入口桥接（NetworkEntry）。
- 游戏能否在 Wine 中运行取决于 Wine 版本与容器配置，本项目不提供游戏兼容性补丁。

## 构建

由 GitHub Actions（`.github/workflows/build.yml`，运行环境为 `macos-14`）自动构建 universal（Apple Silicon + Intel）应用。推送 `v*` 标签时，会自动发布 Release。

本地构建需要 macOS、Xcode 命令行工具、Go 1.26+、Node.js 22+ 与 pnpm、Zig 0.13：

```bash
bash scripts/build-macos.sh
```

产物为 `dist/Poem.app` 和 `dist/Poem-<版本>-macOS.zip`。

单独开发界面时，可以在浏览器里打开预览模式（不连接 Go 服务）：

```bash
cd frontend && pnpm install && pnpm dev
# 打开 http://localhost:5173/?preview
```

## 项目结构

```
.
├── main.go                 入口：嵌入前端、助手与 Lua 模块，创建窗口
├── cmd/poem-helper/        在 Wine 中运行的助手（GOOS=windows）
├── internal/
│   ├── launcher/           核心服务：设置、启动流程、渠道切换、Lua
│   ├── wine/               Wine 检测、路径换算、命令构建
│   ├── helperproto/        启动器与助手之间的 JSON 协议
│   ├── injector/           进程创建与模块注入（助手使用）
│   ├── netproxy/           私服代理、证书、RSA 替换、网页重定向
│   ├── gamelang/ gamever/  游戏语言与版本文件
│   └── dialog/ window/     文件选择与窗口
├── native/lua-entry/       本地 Lua 桥接（C + MinHook）
├── frontend/               界面（React + Vite）
├── build/darwin/           Info.plist
└── scripts/build-macos.sh  一键构建
```

## 致谢与来源

本项目移植自 Windows 版 Poem Launcher。其中部分代码来自 [Cyrene Launcher](https://github.com/Mar7thDev/CyreneLauncher)（Mar7thDev），Cyrene Launcher 的上游是 Firefly Shelter 的 Firefly Launcher。两者均以 MIT License 发布，原始版权声明见 [LICENSE](LICENSE)。

`native/lua-entry/minhook` 为 [MinHook](https://github.com/TsudaKageyu/minhook) v1.3.4，使用其自带的许可证。

## 许可

[MIT License](LICENSE)。游戏美术资源版权归原权利人所有。
