# 多 Agent 运行时设计

`internal/agent.Runtime` 定义桌面端使用的会话、模型选择、提交、历史、停止、交互和事件接口。`internal/opencode` 将 OpenCode v2 的 HTTP 和 SSE 转成这些类型。工作区记录保存目录和运行时 ID，桌面服务按运行时 ID 路由调用。OpenCode 的 Skills、MCP 和凭据操作当前由它自己的适配器提供。

## Pi 适配器

在 Go 中由独立的 `internal/pi` 适配器启动 `pi --mode rpc --session-dir <path>`。子进程以工作区为当前目录，每个活跃会话拥有独立进程。宿主按行持续读取 stdout，并用请求 ID 关联命令响应。stderr 写诊断日志。使用 Pi 自己的配置和凭据。参考 [Pi RPC 协议](https://docs.pi.dev/latest/rpc)与 [命令参考](https://docs.pi.dev/latest/rpc-commands)。

| 桌面动作 | Pi 适配器 |
| --- | --- |
| 创建和恢复 | 调用 `new_session` 获取会话，通过 `get_state` 取得稳定的 session 文件，恢复时通过 `switch_session` 加载 |
| 模型选择 | 用 `get_available_models` 构建列表，选择后发 `set_model`，可选的推理强度用 `set_thinking_level` |
| 发送与停止 | 用 `prompt` 提交，用 `abort` 停止。已接受的请求等 `agent_settled` 决定是否结束，有 `handled` 响应时直接完成 |
| 历史与增量 | 用 `get_messages` 重新读快照，将 `message_update` 和工具事件映射到标准化消息和 Part |
| 交互 | 将 Pi 扩展明确发出的交互请求转成表单，并用匹配原请求 ID 的 `extension_ui_response` 回复 |

Pi 的交互请求来自扩展，OpenCode 的权限请求来自引擎。Pi 适配器只有在工具扩展确实提供可阻塞的审批时才启用统一的权限控件。模型变体、文件差异、附件类型、Skill 附加及 MCP 由运行时能力分别声明。需要 Pi 自身管理会话文件的迁移时，使用稳定的桌面 ID 指向该文件，避免让渲染器读写 Pi 日志。

## 同时使用多个内核

1. 用运行时 ID、运行时实例 ID、工作区 ID 和不透明的引擎会话 ID 标识会话与事件。实例 ID 用于区分本地进程、Pi 独立会话或未来远端连接。工作区的运行时字段作为新任务默认值。会话列表合并各个运行时的结果，并按各自的游标继续分页。
2. 新建任务时选择 OpenCode 或 Pi，任务继续、停止、交互回复和归档都回到原引擎。失败重连时重取历史与待处理交互。已经接受的提示由引擎掌管，桌面端避免自动重发。
3. 生命周期按实例管理。Go 进程注册表保留正在执行的实例，在一个任务结束后回收闲置 Pi 进程。关闭应用时先请求退出，再按超时终止进程组。
4. 多内核之间复用同一个工作区时，提示用户查看正在运行的任务和文件变化。要并行修改同一 Git 项目，可给任务选择独立 worktree。
