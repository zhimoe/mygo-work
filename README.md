# MyGo Work

MyGo Work 是一个面向本地文件和项目的 macOS Agent 工作台。它使用 MyGo 的 WKWebView 作为桌面界面，由 Go 管理 OpenCode v2 进程、工作区和模型交互。界面参考 [OpenWork MIT 桌面版](https://github.com/different-ai/openwork/tree/v0.18.68)。

[下载 macOS 版本](https://github.com/zhimoe/mygo-work/releases/latest) · [运行时与多 Agent 设计](docs/agents.md)

## 安装与开始使用

1. 在 Releases 下载 Apple Silicon 版 DMG，打开后将 **MyGoWork.app** 拖进「应用程序」。
2. 打开 MyGo Work，选择「添加工作区」，打开一个本地文件夹。
3. 新建任务，从输入框下方选择模型和智能体，输入任务并点击「运行任务」。模型按 OpenCode 配置提供，也可以在「设置 → 模型供应商」添加 API 密钥。
4. 在侧栏回看任务，在右侧面板查看工作区文件、文件变更、技能与 MCP 连接。

应用默认显示简体中文。在「设置 → 界面语言」可以即时切换到 English，选择会在下次启动时保留。

发布包内置 OpenCode 2.3.2。本版在 Apple Silicon macOS 27.2 上验证。发行包使用 ad hoc 签名，尚未经过 Apple 公证，Gatekeeper 可能拒绝从其他 Mac 下载的包。也可以在本机从源码构建。

## 功能

| 功能 | 可用操作 |
| --- | --- |
| 工作区与会话 | 添加本地文件夹，新建、搜索、重命名、归档、恢复和删除会话 |
| 任务与模型 | 选择模型、智能体与推理强度，附加文件，查看流式回复和工具步骤，停止任务 |
| 交互与权限 | 回答引擎发来的问题，对工具请求选择允许一次、始终允许或拒绝 |
| 文件与变更 | 查看文件树、Markdown、CSV、TSV 和会话差异，编辑文本或表格，在访达中定位文件 |
| 技能与连接 | 浏览工作区技能并附加到任务，添加和管理远程 MCP，连接支持 API 密钥的模型供应商 |

| 快捷键 | 操作 |
| --- | --- |
| ⌘N | 新建任务 |
| ⇧⌘F | 搜索会话 |
| ⌘, | 打开设置 |
| Enter | 提交当前任务 |
| ⇧Enter | 在输入框换行 |

## 从源码运行

需要 macOS、Git 和 Go 1.27.3 或更新版本。

```sh
git clone https://github.com/zhimoe/mygo-work.git
cd mygo-work
MYGO_WORK_DATA_DIR="$HOME/Library/Application Support/MyGo Work Pi" ./scripts/install-opencode.sh
go run .
```

构建可分发的 DMG，或仅构建应用。

```sh
./scripts/build-macos.sh
./scripts/build-macos.sh -skip-dmg
```

输出位于 `dist/darwin-arm64/`。Intel Mac 的构建路径为 `dist/darwin-amd64/`。构建脚本下载固定版本的 OpenCode v2，校验 SHA-512 后随应用打包。配置 Developer ID 和公证前，构建默认使用 ad hoc 签名。

## Pi 实验分支

`experimental/pi-agent` 从 `v0.1.0` 建立。它支持在同一个本地工作区里切换 OpenCode v2 和 Pi Coding Agent。

1. 安装 [Pi Coding Agent](https://pi.dev/)，运行 `pi` 后使用 `/login` 或配置本地模型。本分支用 Pi 1.5.2 验证。
2. 从此分支启动 MyGo Work。在工作区名称旁的「更多 → 任务引擎」选择 **Pi Coding Agent**。
3. 新建任务，选择 Pi 的模型。切换引擎后，侧栏加载该引擎自己的会话。切回时可以继续原有任务。

Pi 会话支持流式回复、工具过程、模型与推理强度选择、停止、重启恢复、归档和常用扩展交互。附件以本地路径交给 Pi。Pi 使用自身配置的模型、凭据、技能、MCP 和权限。桌面的 OpenCode 连接管理、差异面板和工具授权面板适用于 OpenCode 工作区。

`MYGO_WORK_PI` 可以指定本机 Pi 的绝对路径。应用也会搜索 `PATH`、`~/.local/bin` 和 Pi 的默认安装位置。Pi 继续读取其常规配置目录，桌面会话记录存储在应用数据目录的 `pi/` 子目录。此分支默认使用独立的 `MyGo Work Pi` 应用数据目录。

## 本地数据与配置

| 路径或变量 | 用途 |
| --- | --- |
| `~/Library/Application Support/MyGo Work Pi` | 此分支默认应用数据目录，记录工作区、语言、连接和 OpenCode 会话与凭据 |
| `MYGO_WORK_DATA_DIR` | 指向独立的应用数据目录 |
| `MYGO_WORK_OPENCODE` | 指定已有的 OpenCode v2 可执行文件 |
| `XDG_CONFIG_HOME` | 为 OpenCode 指定单独的全局配置根目录 |

OpenCode 会读取它支持的全局和项目配置。桌面应用管理的引擎仅监听回环地址，使用每次启动生成的密码。模型请求由 OpenCode 发往所选供应商。使用内置执行模式时，文件修改、Shell 和外部目录访问会经过授权。规划模式和自定义智能体使用各自的 OpenCode 权限规则。

## 测试

```sh
go test ./...
go vet ./...
MYGO_WORK_TEST_OPENCODE="$(./scripts/install-opencode.sh)" \
  go test -v -run TestMacOSDesktop .
```

集成测试在真实的 MyGo WKWebView 和 OpenCode v2 进程中运行，由本地测试模型提供确定性响应。测试使用临时工作区，覆盖中英文切换、文件编辑、MCP 重连、流式输出、工具授权、停止、归档及重启恢复。

Pi 的原生桌面测试通过本机 Pi 进程调用本地模型和测试扩展，覆盖工作区切换、工具调用、模型选择、流式输出、停止、扩展确认，以及重启后恢复。

```sh
MYGO_WORK_TEST_PI="$(command -v pi)" \
MYGO_WORK_TEST_OPENCODE="$(./scripts/install-opencode.sh)" \
  go test -v -run TestMacOSPiDesktop .
```

## 运行时与许可

桌面界面使用统一的会话、消息和交互类型。OpenCode v2 适配器负责 HTTP 与 SSE 转换。Pi 适配器通过 JSONL RPC 实现。详见[多 Agent 运行时设计](docs/agents.md)。

第三方许可见 [licenses](licenses/)。
