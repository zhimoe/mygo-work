# MyGo Work

MyGo Work 是一个参考 [OpenWork MIT 桌面版](https://github.com/different-ai/openwork/tree/v0.18.68)的本地 Agent 工作台。界面运行在 MyGo 的 macOS WKWebView 中，Go 宿主管理 OpenCode v2 子进程和本地工作区。

- 侧栏管理本地文件夹和任务，支持搜索、重命名、归档、恢复和历史回看。
- 任务区支持选择模型和 Agent、添加附件、查看流式回复和工具步骤、停止任务、回答问题和审批工具调用。
- 右侧可查看工作区文件、Markdown、CSV、TSV 和会话文件差异，编辑文本与表格，并在 Finder 中定位文件。
- Skills 面板可查看工作区 Skills 并附加到任务。连接面板可管理远程 MCP。模型设置支持为 OpenCode 的 API Key 提供商添加凭据。

默认显示简体中文。在「设置 → 界面语言」可切换至 English，选择会在重启后保留。

## macOS

Apple Silicon 打包并测试了 Go 1.27.4、MyGo 0.10.0 和 OpenCode 2.3.2。构建要求 Go 1.27.3 或更高版本。下面的构建命令会下载并校验当前 CPU 架构的 v2 CLI，将它随应用打包。应用使用本机 ad hoc 签名。

```sh
./scripts/build-macos.sh -skip-dmg
open "dist/darwin-arm64/MyGoWork.app"
```

也可以从源码运行。

```sh
./scripts/install-opencode.sh
go run .
```

应用默认在 `~/Library/Application Support/MyGo Work` 存储工作区索引、引擎状态、会话及已连接的凭据。它读取 OpenCode 所识别的全局与项目配置。可以用 `MYGO_WORK_DATA_DIR` 切换应用数据目录，用 `XDG_CONFIG_HOME` 为测试指定独立的 OpenCode 全局配置目录，用 `MYGO_WORK_OPENCODE` 指定 v2 可执行文件。引擎仅监听回环地址，使用每次启动生成的密码。

本地端的目录权限、进程和网络能力遵循 OpenCode 与 macOS 的实际权限。使用内置执行模式运行任务时，文件修改、Shell 和外部目录访问会经过 OpenCode 授权。规划模式和自定义 Agent 使用 OpenCode 自身的权限规则。

## 验证

```sh
go test ./...
go vet ./...
MYGO_WORK_TEST_OPENCODE="$PWD/resources/darwin-arm64/opencode" \
  go test -v -run TestMacOSDesktop .
```

macOS 集成用例在实际 MyGo WKWebView 中加载生产界面，用真实 OpenCode v2 调用本地测试模型。它使用临时工作区和应用配置，覆盖了默认中文、中英切换和重启后保留选择，以及工作区文件和表格修改、MCP 重连、流式回复、修改授权、停止和历史恢复。

## 设计

Go 和 UI 之间使用引擎无关的会话、消息和权限类型。Pi 和后续引擎的接入设计见 [多 Agent 运行时设计](docs/agents.md)。

第三方许可见 [licenses](licenses/)。
