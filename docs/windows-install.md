# MyGo Work Windows 安装指南

适用于源码构建的 v0.2.0 Windows amd64（x64）版本。安装包内置 OpenCode 2.3.2，无需另外安装 Node.js 或 npm。

## 1. 需要拷贝什么

构建产物都在 `dist/windows-amd64/`。按使用方式二选一：

| 使用方式 | 需要拷贝 | 大小 |
| --- | --- | --- |
| 安装版（推荐） | 只拷贝 `mygo-work-setup-0.2.0-amd64.exe` 这一个文件 | 约 100 MB |
| 便携版 | 拷贝整个 `dist/windows-amd64/` 文件夹 | 260 MB |

便携版拷贝后可以删除以下内容，不影响运行：

- `mygo-work-setup-0.2.0-amd64.exe`（安装器本身）
- `.mygo-files`（构建元数据）
- `licenses/` 建议保留，用于第三方许可证合规

必须保留且必须在同一目录：

```
MyGoWork.exe
opencode.exe
```

不要只拷贝 `MyGoWork.exe`，也不要只拷贝 `opencode.exe`。应用启动时会在 `MyGoWork.exe` 同目录查找 `opencode.exe`。

> 不要拷贝整个仓库或 `resources/windows-amd64/opencode.exe`。`resources/` 只是构建时的资源来源，不是可直接运行的发行包。

## 2. 系统要求

- Windows 10 / Windows 11，64 位（x64 / amd64）
- Microsoft Edge WebView2 Runtime
  - Windows 11 通常已内置
  - Windows 10 如提示缺失，请安装：[WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
- 安装版约占 280 MB 磁盘空间；便携版约 260 MB
- 安装版为当前用户安装，**不需要管理员权限**
- 使用云端模型时需要网络连接

Windows on ARM 设备可以运行此 amd64 包（系统模拟），但更推荐使用 arm64 版本。

## 3. 安装版

1. 把 `mygo-work-setup-0.2.0-amd64.exe` 拷贝到 Windows 机器。
2. 双击运行。
3. 如果出现「Windows 已保护你的电脑」：
   - 点击「更多信息」→「仍要运行」。
   - 这是因为安装包未做代码签名，属于预期行为。
4. 选择安装目录。默认是：

   ```
   %LOCALAPPDATA%\Programs\MyGoWork
   ```

5. 完成页可以勾选：
   - **运行 MyGo Work**：立即启动
   - **创建桌面快捷方式**：在桌面创建 `MyGoWork` 快捷方式
6. 开始菜单中会出现 `MyGoWork`。

静默安装（不创建桌面快捷方式）：

```bat
mygo-work-setup-0.2.0-amd64.exe /S
```

安装位置、开始菜单快捷方式、卸载项都由安装器自动创建。安装目录下会包含：

```
MyGoWork.exe
opencode.exe
licenses\
Uninstall.exe
```

## 4. 便携版

1. 把便携版文件夹拷贝到任意可写目录，例如：

   ```
   D:\Apps\MyGoWork\
   ```

2. 确认 `MyGoWork.exe` 和 `opencode.exe` 在同一目录。
3. 双击 `MyGoWork.exe` 运行。
4. 如需桌面快捷方式，可右键 `MyGoWork.exe` →「发送到」→「桌面快捷方式」。

如果文件夹是从网络、聊天软件或下载的文件解压得到，文件可能带有 Windows 的「来自网络」标记（Zone.Identifier 附加流），内网杀软和 SmartScreen 会因此更严格。在便携版目录打开 PowerShell 执行：

```powershell
Get-ChildItem -Recurse -File | Unblock-File
```

也可以只解除单个文件：

```powershell
Unblock-File .\opencode.exe
```

安装版由安装器自行写入文件，通常不带这个标记；便携版从网络拷贝后建议先执行上面的命令。

便携版不会写入安装目录以外的程序文件，但用户数据仍保存在 `%APPDATA%\MyGo Work`。

## 5. 首次使用

1. 启动后点击「添加工作区」，选择一个本地文件夹。
2. 在左侧工作区下点击「新建任务」，选择模型并输入需求。
3. 点击右下角齿轮打开设置：
   - 「界面语言」可在简体中文和 English 之间切换。
   - 「模型供应商」可添加 API 密钥（OpenCode 引擎）。
4. 右下角显示当前引擎。点击可在 **OpenCode v2** 和 **Pi Coding Agent** 之间切换。
   - OpenCode 已内置，开箱即用。
   - Pi 不随安装包提供，需要单独安装 Pi CLI；未安装时 Pi 选项不可用。
   - Windows 上默认从 PATH 查找 `pi`，node 由 `pi.cmd` 自己查找，应用不再单独检查 node。也可以设置 `MYGO_WORK_PI` 指向 `pi.exe` 或 `pi.cmd`，此时不要求 PATH 中有 `pi`。

### 本地数据位置

| 内容 | 路径 |
| --- | --- |
| 工作区、语言、连接、会话记录 | `%APPDATA%\MyGo Work` |
| OpenCode 日志 | `%APPDATA%\MyGo Work\engine\opencode.log` |

### 可选环境变量

| 变量 | 用途 |
| --- | --- |
| `MYGO_WORK_DATA_DIR` | 指定独立的应用数据目录 |
| `MYGO_WORK_OPENCODE` | 指定已有的 OpenCode v2 可执行文件 |
| `MYGO_WORK_PI` | 指定本机 Pi CLI 路径 |

## 6. 卸载

安装版：

1. 打开「设置」→「应用」→「已安装的应用」。
2. 找到 `MyGoWork`，点击「卸载」。
3. 或者运行安装目录下的：

   ```
   %LOCALAPPDATA%\Programs\MyGoWork\Uninstall.exe
   ```

卸载会删除程序文件和快捷方式，但**不会删除用户数据**。如需彻底清理：

```bat
rmdir /s /q "%APPDATA%\MyGo Work"
```

便携版直接删除文件夹即可，用户数据同样需要手动删除上述目录。

## 7. 常见问题

**启动提示安装 WebView2**

安装 [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) 后重新打开应用。

**SmartScreen 或杀毒软件警告**

安装包和程序均为未签名构建。确认文件来源可信后，选择「更多信息」→「仍要运行」。

**提示 OpenCode v2 executable unavailable**

- 便携版检查 `opencode.exe` 是否与 `MyGoWork.exe` 同目录。
- 检查 `opencode.exe` 是否被杀毒软件隔离。
- 也可以通过 `MYGO_WORK_OPENCODE` 指定 OpenCode 完整路径。
- 查看日志：`%APPDATA%\MyGo Work\engine\opencode.log`。

**Pi 选项是灰的**

Pi 不包含在安装包内。请先安装 Pi Coding Agent，运行 `pi` 完成 `/login` 或模型配置，然后重新打开应用。Windows 上默认从 PATH 查找 `pi`，node 由 `pi.cmd` 自己查找；如果 Pi 在自定义路径，设置 `MYGO_WORK_PI` 指向 `pi.exe` 或 `pi.cmd`，此时不要求 PATH 中有 `pi`。

**ARM64 Windows 上能用吗**

可以，系统会用 x64 模拟运行，但推荐使用专门的 arm64 安装包以获得更好性能。

## 8. 文件校验

v0.2.0 amd64 构建产物 SHA-256：

| 文件 | SHA-256 |
| --- | --- |
| `mygo-work-setup-0.2.0-amd64.exe` | `95499e21d090a509856dae6a171a310eca2c8fdb95db7baacdb0410e27220ee6` |
| `MyGoWork.exe` | `ebf29e7adbc64f737d9ab2ba20a982fd6233fb64a9e88a5f26ef033e7fda6d7f` |
| `opencode.exe` | `5d52d86e2c3fd2c4d51cefc893f64c9ee66bc2e9535332195802f7c646b2a3a3` |

在 Windows 上校验：

```powershell
Get-FileHash .\mygo-work-setup-0.2.0-amd64.exe -Algorithm SHA256
```
