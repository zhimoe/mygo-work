# 多 Agent 运行时设计

`internal/agent.Runtime` 定义桌面端使用的会话、模型选择、提交、历史、停止、交互和事件接口。`internal/opencode` 将 OpenCode v2 的 HTTP 和 SSE 转成这些类型。`internal/pi` 连接本机 Pi Coding Agent 的 JSONL RPC 进程。

工作区保存目录与当前运行时 ID。打开引擎列表时，前端通过 `PiAvailable` 检测本机 Pi。Pi 不可用时，OpenCode 仍可选。`SetWorkspaceRuntime` 校验目标引擎可运行后保存选择，桌面重新加载并按所选引擎恢复工作区会话、模型和文档入口。

OpenCode 管理自己的会话与凭据。Pi 的进程使用各会话对应的文件，桌面索引保存标题、归档状态、工作区及现有会话文件的相对名称。

## Pi RPC

Pi 适配器按活跃会话建立子进程，并在需要读取历史时重新打开持久化会话。首次启动使用 `--session-id` 指定稳定 ID，恢复时通过 `--session` 打开已有的 JSONL 文件。子进程以工作区为当前目录，使用 Pi 自身的配置和凭据。

| 桌面动作 | 对应 Pi RPC |
| --- | --- |
| 发现模型、读状态 | `get_available_models`、`get_state` |
| 选择模型和推理强度 | `set_model`、`set_thinking_level` |
| 提交和停止 | `prompt`、`abort`。模型执行状态在收到 `agent_settled` 后结束。返回 `handled` 的扩展命令在命令完成时结束 |
| 历史与增量 | `get_messages` 读取快照。用 `message_start` 初始化流式消息，应用 `message_update` 的文字、思考过程和工具增量，最后用引擎的消息快照对齐 |
| 扩展交互 | 将 `select`、`confirm`、`input`、`editor` 转成桌面表单，用相同请求 ID 回传 `extension_ui_response` |

宿主按请求 ID 关联响应。stdout 按行读取，stderr 保存在工作区的引擎日志。退出时先关闭 stdin，等待 Pi 清理，超时后终止进程组。Pi 的桌面交互来自 Pi 扩展，工具权限遵循 Pi 自己的配置。

协议细节见 [Pi RPC](https://docs.pi.dev/latest/rpc)、[RPC 命令](https://docs.pi.dev/latest/rpc-commands)和[扩展 UI](https://docs.pi.dev/latest/rpc-extension-ui)。

## 下一步可支持的并行工作方式

- 将运行时 ID、实例 ID、工作区 ID 和引擎会话 ID 组成会话主键。这样可以在同一个列表展示同一文件夹下的不同内核会话，并保留各自的分页与恢复方式。
- 让引擎通过能力声明提供工具审批、差异、附件、MCP 和技能。工具执行与会话数据仍由各引擎管理。
- 为并行任务提供独立 worktree 选项。让用户在同一工作区查看各任务的运行状态和文件变更。
