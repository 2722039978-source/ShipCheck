# ShipCheck 实现说明

## Windows 操作流程

`ShipCheck.exe` 使用 Walk 创建 Windows 原生窗口。上方接收拖放文件，中间提供五个操作页面，下方显示可复制的报告文本及打开报告、打开目录、转到复跑的按钮。

| 页面 | 输入与操作 | 保存的内容 |
| --- | --- | --- |
| 验收 ZIP | 拖入或选择 JSON 工单和 ZIP，文件齐备后自动验收，也可以点击重新验收。 | 工单、ZIP 副本、验收 JSON、记录 JSON、HTML 报告。 |
| 打包 | 选择工单、暂存目录，填写 ZIP 相对输出路径和命令。 | 完整输入快照、工单、命令与日志、取得的 ZIP、验收结果和报告。 |
| 复跑 | 选择失败记录；复跑使用原输入与原命令；修正尝试使用填写的新输入或新命令。 | 每次独立保存输入快照、执行记录、ZIP、报告和与来源记录的比较结果。 |
| 工单与模板 | 新建、读取、编辑、校验工单，保存为新文件；读取或保存完整模板。 | 完整规则和模板来源。保存工单后，验收及打包页面使用保存的路径。 |
| 历史记录 | 填写工单号、版本、状态和日期条件，选择查询结果。 | 查询已有记录；打开对应报告或结果目录，失败记录可以带入复跑页面。 |

工单编辑页面以上方工单号、版本、平台字段作为基本信息，JSON 编辑区提供完整规则。读取模板时将打包默认值填入打包页面；保存模板时读取该页面的输出路径和命令。工单和模板保存操作拒绝覆盖已有文件。

窗口一次执行一项后台任务。处理期间禁用页面操作，并在记录保存完成后恢复。打包工具在工作副本中执行，标准输出和标准错误写入文件。图形版启动打包进程时使用 Windows 的无控制台窗口选项。

## 源文件职责

| 文件 | 职责 |
| --- | --- |
| `gui_windows.go` | 窗口、拖放、验收、打包、复跑、后台任务和报告按钮。 |
| `gui_orders.go` | 工单 JSON 编辑、模板读取与保存、修正尝试。 |
| `gui_history.go` | 历史筛选、列表选择和打开对应结果。 |
| `main.go`、`cli_extended.go` | CLI 命令、参数校验、文本或 JSON 输出及退出码。 |
| `verify.go` | 工单读取、ZIP 完整读取、路径有效性、文件摘要和发布声明。 |
| `rules.go` | 内容规则、目录策略、组件依赖、迁移顺序及检查范围。 |
| `templates.go` | 严格 JSON 读取、模板校验、完整工单复制、来源摘要和新文件保存。 |
| `workflow.go` | 打包流程、输入快照、成品验收和执行记录。 |
| `workspace.go` | 目录解析、文件复制、工作副本及成品保存。 |
| `command.go` | 工具路径解析、参数传递、进程执行和日志。 |
| `records.go` | 独立验收记录、ZIP SHA-256、总体状态和历史查询。 |
| `revision.go` | 从失败记录创建复跑或修正尝试，比较问题和内容变化。 |
| `reports.go` | 通过 `html/template` 生成可离线打开的报告。 |

CLI 与 GUI 调用相同业务函数。`shipcheck_gui` 构建标记选择图形入口，默认构建选择 CLI。业务处理使用 Go 标准库；窗口使用 Walk，Windows 参数解析和系统打开操作使用 `golang.org/x/sys/windows`。

## 工单与规则

`parseOrderJSON` 使用 `encoding/json.Decoder` 拒绝未知字段、无效对象和额外 JSON 内容，随后调用 `validateOrder`。窗口编辑、命令行读取、模板创建和保存使用同一套规则校验。

基础工单要求 `order_id`、`from_version`、`to_version`、`platform` 为非空字符串，`required_files` 为数组。完整字段和示例见 [工单格式](../README.md#工单格式)。

- `files` 中的 `path` 自动加入必需文件集合。预期摘要、字节数、空内容规则分别由 `sha256`、`size`、`allow_empty` 控制。
- 必需文件集合同时包含 `release.json`、`required_files` 和全部迁移关联文件。
- `allowed_directories` 控制目录；`forbidden_patterns` 使用 `path.Match`，匹配完整路径及父目录；`allow_extra_files: false` 拒收必需文件集合之外的普通文件。
- 全部 ZIP 条目都检查无效路径、重复名称、大小写冲突和文件与目录冲突。普通文件读取到末尾，读取错误、CRC 错误和特殊文件类型导致拒收。
- `release.json` 最大为 1 MiB，版本和平台按字符串精确匹配。实际文件 SHA-256 和字节数保存在 `verification.json`。

未填写的内容规则保持未启用，工单省略 `components` 或 `migrations` 时跳过相应声明检查。`allowed_directories`、`components` 和 `migrations` 使用 `omitzero` 保存，使显式空数组在模板复制及工单保存后保留原意。

组件依赖使用明确的允许版本数组。包内组件与工单要求的名称集合、版本逐项比较，再检查依赖组件的实际版本。迁移步骤必须从起始版本连续到达目标版本，并与工单中的步骤和文件顺序一致。目标环境负责迁移执行、试装和运行验证。

`verification.checks` 按 `archive`、`files`、`metadata`、`components`、`migrations` 记录检查范围，状态使用 `passed`、`rejected`、`not_requested` 或 `not_run`。

## 模板与编辑

模板保存 `name`、`version`、完整 `order`、默认 `output` 和 `command`。默认命令可以省略；提供命令时校验工具、参数及路径。输出必须是工作目录内的有效相对路径，不能与必需输入文件或其目录发生冲突。

`instantiateTemplate` 序列化并复制完整模板，再设置工单号和指定的起始、目标版本。工单的 `template` 字段记录名称、版本和序列化模板的 SHA-256。每份工单保存独立的规则数组与嵌套内容；模板变化不影响已创建的工单。

`saveOrder` 和 `saveTemplate` 在创建文件前完成校验，使用排他创建保护已有路径，写入失败时清理本次文件。预期文件摘要由发布者提供；模板创建保留工单已有规则。

## 记录保存与修正

GUI 结果根目录为可执行文件旁的 `.shipcheck`。CLI 默认为当前工作目录的 `.shipcheck`，通过 `--results` 可以指定其他目录。每份记录保存在独立的 `verify-*`、`run-*`、`replay-*` 或 `revise-*` 目录中。

`verifyAndRecord` 保存原工单，将来源 ZIP 复制到本次 `artifact/`，再检查保存的副本。来源路径和副本 SHA-256 写入 `receipt.json`。无法取得普通文件时记录拒收原因。

`runPackaging` 复制暂存目录到结果目录的 `input/`，从该快照建立临时工作目录，执行命令并保存成品。通过和拒收均保留输入快照。工作目录在处理结束后清理。结果目录必须位于暂存目录之外。

`derivePackaging` 读取失败记录，保持原工单和输出路径，选择原输入或新输入、原命令或新命令。新尝试保存独立的输入快照、执行记录、日志、成品和报告，使用 `source_directory` 关联原记录。结果目录必须位于原证据和当前输入之外。

`receipt.json` 的总体状态同时考虑命令执行和 ZIP 验收。命令返回非零退出码、启动失败或 ZIP 拒收均会生成 `rejected` 总体状态。`verification.json` 保存 ZIP 自身的验收结果。`receipt.json` 还记录 `schema_version`、`tool_version`、UTC 时间 `created_at` 和 `zip_sha256`。

问题比较按问题代码、路径、字段和预期值识别同一项检查，生成 `resolved`、`persisting`、`added` 和 `unverified`。文件读取失败或 ZIP 无法检查时，对相关原问题保留无法确认状态。输入和 ZIP 内容按路径、类型、字节数和 SHA-256 比较，生成 `added`、`removed`、`modified` 记录；ZIP 无法完整比较时保存说明。

`report.html` 使用 `html/template` 转义工单与错误文本。报告包含检查范围、文件摘要、完整规则、执行信息及比较结果，并用相对链接访问记录文件、日志、输入快照和 ZIP。转移完整结果目录后，可以离线阅读这些内容。

## 历史查询

`listHistory` 读取结果根目录的直接子目录。工单号精确匹配，版本匹配起始或目标版本，状态使用 `passed` 或 `rejected`。创建时间保存为 UTC；日期条件按当前机器时区解释，开始和结束日期均包含当天。

新记录从 `receipt.json` 读取。已有的 `order.json`、`verification.json` 和可选 `execution.json` 仍可用于查询旧记录。旧记录缺少创建时间时显示说明，日期筛选将其排除。无效记录返回文件位置和错误原因。

## 构建与验证

`build.ps1` 使用 Go 1.24 或更新版本，将 Windows 应用清单编译进程序，生成 `ShipCheck.exe` 与 `.local\shipcheck.exe`。依赖缓存、构建缓存和构建临时文件位于 `.local`。

从项目根目录执行：

```powershell
& .\test.ps1 -Integration
```

该命令运行全部 Go 测试、CLI 和 GUI 的 `go vet`、两个入口的构建，以及 `tests/accept-extended.ps1`。Go 测试使用真实文件和 ZIP，覆盖内容摘要、大小、空文件、目录规则、组件依赖、迁移顺序、记录保存、模板独立复制、修正与复跑。集成脚本通过本机 `tar.exe` 验证完整交付、错误摘要、缺少迁移文件、修正成功、原证据保持原样、报告摘要和历史过滤。

原生窗口检查独立执行：

```powershell
& .\tests\gui-extended.ps1 -Evidence 'E:\path\to\failed-run'
```

`-Evidence` 需要示例 `1.6 → 1.7` 工单的失败记录，输入快照包含完整的 `examples/staging` 内容，输出路径为 `upgrade.zip`，原 ZIP 缺少迁移文件。脚本通过 `tests/NativeUI.cs` 调用 Windows 原生 API，读取控件文本、触发按钮及文件对话框，检查工单保存、模板读取、修正和历史查询。脚本结束时输出结果目录；执行结果以本次输出和生成文件为依据。

`.local` 中保留的 `accept.ps1`、`check-failures.ps1`、`gui-accept.ps1` 和 `inspect-gui.ps1` 依赖本机准备的示例路径与辅助文件，用于对应场景的补充检查。维护中的测试入口为 `test.ps1` 和 `tests/` 目录。测试生成内容位于 `.local`；GUI 实际业务记录写入可执行文件旁的 `.shipcheck`。
