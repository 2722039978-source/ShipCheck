# ShipCheck

## Windows 窗口

双击 `ShipCheck.exe`，在窗口中选择需要的操作。

- 「验收 ZIP」：拖入 JSON 工单和 ZIP，或者点击「选择工单」「选择 ZIP」。文件齐备后自动验收，并保存工单、实际检查的 ZIP 副本和报告。
- 「打包」：选择工单和暂存目录，填写 ZIP 相对输出路径及打包命令，点击「打包并验收」。命令执行成功且 ZIP 验收通过时显示交付包路径。
- 「复跑」：选择失败记录，点击「复跑此证据」使用原输入和原命令。填写「修正输入目录」或「修正打包命令」后，点击「创建修正尝试」；空白项目使用原记录中的内容。
- 「工单与模板」：新建或读取工单，编辑工单号、版本、平台和完整 JSON 规则，点击「校验工单」。点击「保存工单并使用」保存到新文件，验收和打包页面随后使用此工单。读取模板会载入完整规则及打包默认值；保存模板使用当前工单与「打包」页面的默认输出路径和命令。
- 「历史记录」：按工单号、起始或目标版本、状态和日期查询。选择记录后，可以打开 HTML 报告或结果目录；失败的打包记录可以转到复跑页面。

窗口下方的报告文本可以复制。「打开验收报告」打开可离线阅读的 `report.html`。窗口记录保存在 `ShipCheck.exe` 所在目录的 `.shipcheck` 中，每次操作使用独立目录。

## 构建与测试

Windows x64 图形版使用 Go 1.24 或更新版本和 Walk 原生控件。将 Go 工具链解压到项目的 `.local\go` 后，在项目根目录用 PowerShell 执行：

```powershell
& .\build.ps1
```

构建生成根目录的 `ShipCheck.exe` 和命令行程序 `.local\shipcheck.exe`。`build.ps1 -Go PATH` 可以指定 Go 可执行文件。依赖、构建缓存和构建临时文件位于 `.local`。图形版可以作为单个 `ShipCheck.exe` 发布，用户选择的打包工具及其运行依赖由本机提供。

```powershell
& .\test.ps1
& .\test.ps1 -Integration
```

`test.ps1` 执行全部 Go 测试、GUI 模板命令测试及 CLI、GUI 两种构建标记的 `go vet`。`-Integration` 另外构建两个入口，并运行 `tests/accept-extended.ps1`，检查真实 ZIP、报告摘要、修正尝试、原证据内容、模板和历史查询。测试文件保存在 `.local`。已执行的验证记录见 [验证记录](docs/validation.md)。

原生窗口检查单独执行，需要一个缺少迁移脚本的失败打包记录：

```powershell
& .\tests\gui-extended.ps1 -Evidence 'E:\path\to\failed-run'
```

此记录应使用示例 `1.6 → 1.7` 工单，`input/` 包含 `release.json`、`app`、`dependencies` 和 `migrations`，预期输出为 `upgrade.zip`。脚本操作实际 Windows 控件，检查工单保存、模板读取、修正尝试和历史查询，并将文件写入 `.local\gui-extended-*`。详细结构和验证方法见 [实现说明](docs/implementation.md)。

## 工单格式

[examples/order.json](examples/order.json) 是可以直接配合 [examples/staging](examples/staging) 使用的基础工单。JSON 对象必须包含以下字段：

| 字段 | 要求 |
| --- | --- |
| `order_id` | 非空工单号字符串。 |
| `from_version` | 非空起始版本字符串。 |
| `to_version` | 非空目标版本字符串。 |
| `platform` | 非空平台字符串，例如 `windows-amd64`。 |
| `required_files` | 必需文件路径数组，可以使用 `[]`。 |

文件路径从 ZIP 根目录开始，使用 `/` 分隔。必需文件按完整路径和大小写匹配。工单和 ZIP 中的路径必须能够用于 Windows；目录穿越、保留设备名称、大小写冲突、文件与目录冲突均会被拒绝。ZIP 全部条目参与路径和重复名称检查，全部普通文件读取到末尾，并记录实际 SHA-256 和字节数。

ZIP 根目录必须包含 `release.json`，内容最大为 1 MiB。它声明 `from_version`、`to_version` 和 `platform`，这些值需要与工单一致。工单和模板拒绝未知 JSON 字段、类型错误和尾随内容。

### 文件内容与目录规则

[examples/strict-order.json](examples/strict-order.json) 展示完整规则；其配套声明为 [examples/strict-release.json](examples/strict-release.json)。该示例要求 `payload/message.txt` 的内容为 UTF-8 文本 `hello`，共 5 个字节，末尾没有换行，并要求两份迁移文件。

| 可选字段 | 含义及默认行为 |
| --- | --- |
| `files` | 每项包含 `path`，以及可选的 `sha256`、`size`、`allow_empty`。列入此数组的文件自动成为必需文件。 |
| `files[].sha256` | 发布者提供的预期 SHA-256，填写 64 个十六进制字符。省略时只记录实际摘要。 |
| `files[].size` | 预期字节数，必须为非负整数。省略时只记录实际大小。 |
| `files[].allow_empty` | `false` 拒收空文件；省略或 `true` 允许空文件，仍须满足已填写的摘要和大小规则。 |
| `allowed_directories` | 允许的目录及其全部子目录，例如 `["app", "dependencies", "migrations"]`。`"."` 允许根目录的普通文件。省略时不限制目录；`[]` 只允许根目录的 `release.json`。 |
| `forbidden_patterns` | 禁止路径规则数组。对完整路径及其父目录匹配，忽略大小写。例如 `app/*.bak` 禁止 `app` 直属的 `.bak` 文件，`cache` 禁止根目录的 `cache` 及其内容。 |
| `allow_extra_files` | `false` 拒收清单之外的普通文件。清单包含 `release.json`、`required_files`、`files[].path` 和迁移关联文件。省略或 `true` 允许额外文件。 |

`forbidden_patterns` 使用 Go `path.Match` 规则：`*` 匹配不含 `/` 的任意字符，`?` 匹配一个非 `/` 字符，字符集合使用 `[]`。目录需要按实际层级填写，`**` 与 `*` 使用相同的路径分隔限制。

预期文件摘要应来自已经确认的发布文件或发布清单。ShipCheck 按工单中提供的预期值进行比较；报告中的实际摘要用于识别本次检查的内容。

### 组件、依赖与迁移

工单可以增加 `components` 和 `migrations`，包内 `release.json` 提供对应的实际声明。

`components` 每项包含 `name`、`version` 和可选的 `dependencies`。依赖项包含 `name` 与非空的 `allowed_versions` 数组，依赖的组件必须同时在工单中声明。组件名称和版本按字符串精确比较，允许版本为明确列出的值。包内 `components` 每项包含 `name`、`version`；声明的组件名称集合和版本需要满足工单，依赖组件的实际版本需要属于 `allowed_versions`。

`migrations` 每项包含 `id`、`from_version`、`to_version` 和非空的 `files` 数组。步骤按数组顺序，从工单起始版本连续到达目标版本；步骤标识与经过的版本不能重复。`release.json` 中的步骤数量、标识、版本、文件列表及其顺序需要与工单一致。关联文件自动成为必需文件。

省略 `components` 或 `migrations` 时，相应声明检查显示为 `not_requested`。显式填写 `components: []` 要求包内没有声明组件。显式填写 `migrations: []` 要求起始版本等于目标版本。现有只包含基本字段与 `required_files` 的工单可以继续使用；其未声明的内容、组件、依赖和迁移规则保持未启用状态。

报告检查文件内容、组件声明、依赖允许版本和迁移顺序。迁移执行效果及程序运行情况由目标环境试装验证。

## 报告与历史记录

`verify`、`run`、`replay`、`revise` 为每次处理创建独立结果目录。通过和拒收的记录都保留；打包命令失败时，总体状态为 `rejected`。

| 文件或目录 | 内容 |
| --- | --- |
| `order.json` | 本次使用的完整工单。 |
| `verification.json` | ZIP 验收结果、拒收原因、检查范围、文件 SHA-256 和字节数。 |
| `receipt.json` | 记录格式版本、ShipCheck 版本、UTC 创建时间、操作类型、总体状态、工单和 ZIP SHA-256。复跑和修正记录包含来源目录及比较结果。 |
| `report.html` | 可离线打开的报告，包含以上信息并链接到本目录内的记录文件和 ZIP。 |
| `artifact/` | 本次保存并实际检查的 ZIP。没有取得文件时，报告说明原因。 |
| `input/` | `run`、`replay`、`revise` 使用的独立输入快照，通过和拒收均保留。 |
| `execution.json` | 打包工具、解析后的路径、参数、工作目录、输出路径、退出码和启动错误。未取得退出码时为 `null`。 |
| `stdout.log`、`stderr.log` | 打包命令的标准输出和标准错误。 |

独立验收保存 ZIP 副本后检查该副本。打包从输入快照建立工作副本，在命令执行结束后保存成品，并验收保存的文件。工作副本在任务结束后清理。需要转移报告时，复制整个结果目录可保留报告内的相对链接。

复跑与修正记录列出已解决、持续存在、新增和无法确认的问题，同时比较输入内容与 ZIP 内容的新增、删除和修改。原记录与原工单保持原样。ZIP 无法完整读取时，报告说明内容比较不可用。

## 命令行

以下命令中的 `shipcheck` 指 `.local\shipcheck.exe`。`verify`、`run`、`replay`、`revise` 和 `history` 的 `--results` 默认为当前目录下的 `.shipcheck`。打包结果目录必须位于输入目录之外，复跑与修正结果目录必须位于原证据目录之外。

### 验收、打包、复跑与修正

```text
shipcheck verify --order ORDER.json --zip PACKAGE.zip [--results DIR]
shipcheck run --order ORDER.json --staging DIR --output RELATIVE.zip [--results DIR] -- TOOL [ARG ...]
shipcheck replay --evidence DIR [--results DIR]
shipcheck revise --evidence DIR [--staging DIR] [--results DIR] [-- TOOL ARG ...]
```

`--output` 使用 `/` 分隔，指向工作副本内尚未使用的 ZIP 路径。工具名从本机 `PATH` 查找，绝对路径指向本机程序，带目录的相对路径从工作副本查找。参数直接传给工具。运行脚本时需要指定相应解释器，例如 `powershell.exe -File .\package.ps1`。

`replay` 使用失败记录的输入快照、工单、输出路径和命令。`revise` 至少提供新输入目录或新命令之一，工单和输出路径继续使用原记录。复跑和修正依赖当前机器可用的工具与环境。

退出码 `0` 表示操作成功或交付通过，`1` 表示打包命令失败或 ZIP 被拒收，`2` 表示参数、工单、证据或记录保存出现错误。

### 历史查询

```text
shipcheck history [--results DIR] [--order-id ID] [--version VERSION] [--status passed|rejected] [--from-date YYYY-MM-DD] [--to-date YYYY-MM-DD] [--json]
```

工单号精确匹配，版本匹配起始版本或目标版本。日期使用当前机器的本地日期，起止日期均包含当天。查询读取结果目录的直接子目录，按记录时间从新到旧排列。`--json` 输出结构化结果，包括报告目录和 ZIP 路径。没有创建时间的旧记录可以按工单号、版本和状态查询，日期筛选会排除这些记录。

### 工单和模板

```text
shipcheck order validate --order ORDER.json
shipcheck template save --name NAME --version VERSION --order ORDER.json --output TEMPLATE.json [--zip-output PACKAGE.zip] [-- TOOL ARG ...]
shipcheck order create --template TEMPLATE.json --id ID [--from VERSION] [--to VERSION] --output ORDER.json
```

模板包含 `name`、`version`、完整 `order`、默认 `output` 和 `command`。`template save` 的 `--zip-output` 默认为 `upgrade.zip`，命令可以省略。[examples/template.json](examples/template.json) 可以直接用于示例交付。

`order create` 完整复制模板规则，保存 `template.name`、`template.version`、`template.sha256`。来源摘要根据序列化后的完整模板计算，包含规则和打包默认值。已创建的工单独立保存，模板后续变化不影响它。`--from`、`--to` 省略时保留模板版本；指定版本仍需满足模板内的迁移规则。保存工单和模板都要求使用新文件路径。

## 演示 1.6 → 1.7

从项目根目录使用 PowerShell 执行。以下示例使用本机 `tar.exe`。

```powershell
$cli = '.\.local\shipcheck.exe'
$order = '.\examples\order.json'
$staging = '.\examples\staging'
& $cli run --order $order --staging $staging --output upgrade.zip -- tar.exe -a -cf upgrade.zip release.json app dependencies migrations
```

通过时，「交付包」后面是已验收 ZIP 的路径。将工单与该 ZIP 拖入窗口「验收 ZIP」页面可以建立独立验收记录。

省略迁移目录生成失败记录：

```powershell
$failed = & $cli run --order $order --staging $staging --output upgrade.zip -- tar.exe -a -cf upgrade.zip release.json app dependencies
$runCode = $LASTEXITCODE
$failed
if ($runCode -ne 1) { throw "预期拒收，实际退出码 $runCode" }
$evidence = ($failed | Where-Object { $_ -like '证据目录 *' }).Substring('证据目录 '.Length)
& $cli replay --evidence $evidence
& $cli revise --evidence $evidence -- tar.exe -a -cf upgrade.zip release.json app dependencies migrations
```

复跑记录继续报告缺少 `migrations/1.6_to_1.7.sql`。修正命令包含迁移目录，报告会列出已解决的问题和 ZIP 内容变化。原失败记录仍可查询：

```powershell
& $cli history --order-id OFFLINE-001 --status rejected
& $cli order create --template .\examples\template.json --id OFFLINE-002 --output .\.local\delivery-002.json
& $cli order validate --order .\.local\delivery-002.json
```
