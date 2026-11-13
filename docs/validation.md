# ShipCheck 0.2.0 验证记录

## 2026-11-09 历史验证摘要

下表转录维护者当时记录的结果。原始日志、报告和窗口产物尚未公开，历史运行的源码提交、Windows 版本、Go 版本和 PowerShell 版本也未随记录提供。独立复核请使用后面的步骤，并按实际运行填写新记录。 [原始验证记录](https://github.com/2722039978-source/ShipCheck/blob/ff93f7075763904828c20046880e95cf12b3de4a/docs/validation.md)

| 检查 | 维护者当时记录的结果 | 公开复核入口 |
| --- | --- | --- |
| Go 测试、GUI 命令测试、CLI/GUI 的 `go vet` 和构建 | 40 个顶层测试及其子测试通过，两种配置的 `go vet` 和构建通过 | [测试脚本](../test.ps1)、[构建脚本](../build.ps1) |
| CLI 完整流程和失败场景 | 正常打包、独立验收、缺件拒收、原命令复跑和修正通过预期检查。版本不符、输入缺件和命令失败触发拒收 | [CLI 集成脚本](../tests/accept-extended.ps1)、[补充失败场景](#补充失败场景) |
| 工单、模板、修正和历史选择 | 窗口中验证通过 | [原生窗口脚本](../tests/gui-extended.ps1) |
| 拖放验收、窗口打包、缺件拒收、证据拖放和点击复跑 | 窗口回归记录为通过，原始记录未公开 | [人工窗口检查](#人工窗口检查) |
| `gofmt`、行尾空白、独立代码审查 | 格式检查和空白检查记录为通过。代码审查记录为未发现交付阻塞项，审查材料未公开 | [可复查的格式检查](#复现自动检查) |

原文记录使用真实 ZIP、文件、本机 `tar.exe` 和 Windows 控件。正常交付的 ShipCheck CLI 退出码为 `0`，预期拒收和打包失败为 `1`。原文记载复跑或修正保留了原失败工单、打包命令、输入与 ZIP 的内容。修正 HTML、同次记录 JSON、窗口保存的工单及模板原件均未公开。

### 历史二进制摘要

| 程序 | SHA-256 |
| --- | --- |
| `ShipCheck.exe` | `BD6DC77104DF0FB3AA85608B6A285B2E51327DA56C59FCD5C244988E711BC9AE` |
| `shipcheck.exe` | `05FCE7F9BBB490716024D7BDEBCD5201E0A0A65EF013FDBD99AF2F1A8B0E0333` |

这两个摘要标识原记录中的产物。复跑应计算新生成的程序和示例 ZIP 的摘要，并记录对应的源码提交和工具链。

## 复现自动检查

使用 Windows x64、PowerShell 7、Windows amd64 版 Go 1.24 或更新版本，确认 `tar.exe` 和 `cmd.exe` 可用。以下提交是公开源码的复现基线。2026-11-09 实际运行所用的提交尚待确认。

```powershell
git clone -c core.autocrlf=false https://github.com/2722039978-source/ShipCheck.git
Set-Location ShipCheck
git checkout --detach ff93f7075763904828c20046880e95cf12b3de4a
git rev-parse HEAD
```

将官方 Go ZIP 解压至 `.local`，使可执行文件位于 `.local\go\bin\go.exe`。首次运行要能下载 Go 依赖。记录 Windows 版本及内部版本号，并在同一个 PowerShell 7 会话运行以下命令。

```powershell
$PSVersionTable.PSVersion.ToString()
& .\.local\go\bin\go.exe version
& .\.local\go\bin\go.exe env GOHOSTOS GOHOSTARCH
tar.exe --version
Get-Command cmd.exe
& .\test.ps1 -Integration
```

| 阶段 | 复核成功的判据 |
| --- | --- |
| `go test -count=1 -v ./...` | 基线源码中有 37 个默认配置的顶层测试。各测试和子测试通过，命令退出码为 `0`。 |
| `go test -tags shipcheck_gui -run TestGUICommand -count=1 -v ./...` | 命中 3 个顶层测试并通过，命令退出码为 `0`。覆盖 GUI 打包命令解析、模板命令和工作目录工具执行。 |
| 两次 `go vet` | 默认配置和 `-tags shipcheck_gui` 配置均以 `0` 退出。 |
| `build.ps1` | 构建完成并列出根目录 `ShipCheck.exe` 和 `.local\shipcheck.exe`。 |
| `tests/accept-extended.ps1` | 结束时输出 `EXTENDED_ACCEPTANCE_PASSED` 和本次结果目录。正常打包、独立验收、修正返回 `0`。摘要不符、缺迁移文件和按原命令复跑返回 `1`。脚本检查 ZIP 的 SHA-256、修正来源、原记录指定文件的摘要、模板和拒收历史。 |

仅执行前两组 Go 测试和两次 `go vet` 可使用 `& .\test.ps1`。以上检查可在 Windows 命令行完成。原生窗口另按下文验证。集成产物写到 `.local\extended-*`，其中 `results` 下每次操作有独立目录，可查看 `report.html`、`receipt.json` 和 `verification.json`。

格式与行尾空白可另行复查。上面的克隆命令保留源码原有换行符。预期 `gofmt -l` 没有输出且退出码为 `0`。`git grep` 退出码为 `1` 表示未匹配到行尾空白，`0` 会列出匹配行。

```powershell
$goFiles = @(git ls-files -- '*.go')
& .\.local\go\bin\gofmt.exe -l @goFiles
git grep -n -I -E '[[:blank:]]+$' HEAD -- '*.md' '*.ps1'
```

### 补充失败场景

下面的命令使用已构建的 CLI 和仓库示例。结果写入单独的本地目录。`test.ps1 -Integration` 已检查 ZIP 缺迁移文件的情形。这里额外覆盖版本声明不符、暂存输入已缺文件、外部打包命令非零退出。

```powershell
$PSNativeCommandUseErrorActionPreference = $false
$cli = '.\.local\shipcheck.exe'
$order = '.\examples\order.json'
$staging = '.\examples\staging'
$cases = Join-Path $PWD.Path ('.local\cases-' + [guid]::NewGuid().ToString('N'))
$good = Join-Path $cases 'good'
& $cli run --order $order --staging $staging --output upgrade.zip --results $good -- tar.exe -a -cf upgrade.zip release.json app dependencies migrations
if ($LASTEXITCODE -ne 0) { throw '准备正常 ZIP 失败' }
$zip = (Get-ChildItem -LiteralPath $good -Recurse -File -Filter 'upgrade.zip').FullName

$wrongOrder = Join-Path $cases 'wrong-version.json'
$wrong = Get-Content -LiteralPath $order -Raw | ConvertFrom-Json
$wrong.to_version = '1.8'
$wrong | ConvertTo-Json | Set-Content -LiteralPath $wrongOrder -Encoding utf8NoBOM
& $cli verify --order $wrongOrder --zip $zip --results (Join-Path $cases 'version')
if ($LASTEXITCODE -ne 1) { throw '版本不符应拒收' }

$partial = Join-Path $cases 'partial-staging'
New-Item -ItemType Directory -Path $partial | Out-Null
Copy-Item -LiteralPath @("$staging\release.json", "$staging\app", "$staging\dependencies") -Destination $partial -Recurse
& $cli run --order $order --staging $partial --output upgrade.zip --results (Join-Path $cases 'missing-input') -- tar.exe -a -cf upgrade.zip release.json app dependencies
if ($LASTEXITCODE -ne 1) { throw '输入缺件应拒收' }

& $cli run --order $order --staging $staging --output upgrade.zip --results (Join-Path $cases 'command-failure') -- cmd.exe /c exit /b 7
if ($LASTEXITCODE -ne 1) { throw '打包命令失败应拒收' }
```

| 结果目录 | 应查看的本次输出 |
| --- | --- |
| `version/verify-*` | `verification.json` 中有 `metadata_mismatch`，字段为 `to_version`。 |
| `missing-input/run-*` | `verification.json` 中有 `missing_file`，路径为 `migrations/1.6_to_1.7.sql`，`input_present` 为 `false`。 |
| `command-failure/run-*` | `execution.json` 中 `exit_code` 为 `7`，`receipt.json` 的总体 `status` 为 `rejected`。 |

## Windows 原生窗口自动检查

构建后，在交互式 Windows 桌面打开 PowerShell 7。用完整示例输入打包，但在命令中省略 `migrations`。这样失败记录的输入快照仍有迁移脚本，修正命令可以把它放入 ZIP。

```powershell
$PSNativeCommandUseErrorActionPreference = $false
$cli = '.\.local\shipcheck.exe'
$results = Join-Path $PWD.Path ('.local\gui-fixture-' + [guid]::NewGuid().ToString('N'))
& $cli run --order .\examples\order.json --staging .\examples\staging --output upgrade.zip --results $results -- tar.exe -a -cf upgrade.zip release.json app dependencies
if ($LASTEXITCODE -ne 1) { throw '预期 ZIP 缺迁移脚本并被拒收' }
$evidence = (Get-ChildItem -LiteralPath $results -Directory -Filter 'run-*').FullName
$record = Get-Content -LiteralPath (Join-Path $evidence 'verification.json') -Raw | ConvertFrom-Json
$missing = $record.issues | Where-Object {
    $_.code -eq 'missing_file' -and $_.path -eq 'migrations/1.6_to_1.7.sql' -and $_.input_present -eq $true
}
if ($record.status -ne 'rejected' -or !$missing) { throw '拒收记录与窗口检查输入不符' }
& .\tests\gui-extended.ps1 -Evidence $evidence
```

预期输出 `GUI_ORDER_AND_TEMPLATE_PASSED`、`GUI_REVISION_PASSED`、`GUI_HISTORY_QUERY_PASSED` 以及最终的 `GUI_EXTENDED_ACCEPTANCE_PASSED`。脚本操作原生控件和文件对话框，验证工单保存、模板读取、修正和历史记录选择，并检查报告按钮可用。工单和模板输出到 `.local\gui-extended-*`，窗口业务记录写在程序旁的 `.shipcheck`。 [窗口脚本](../tests/gui-extended.ps1) · [Windows 控件辅助代码](../tests/NativeUI.cs)

### 人工窗口检查

启动本次构建的 `ShipCheck.exe`，从资源管理器拖放文件。使用 [示例工单](../examples/order.json) 和 [暂存目录](../examples/staging)。每项分别记录实际现象和证据。

| 编号 | 操作 | 预期 |
| --- | --- | --- |
| WIN-01 | 在「打包」选择示例工单和暂存目录，输出填 `upgrade.zip`，命令填 `tar.exe -a -cf upgrade.zip release.json app dependencies migrations`，点击「打包并验收」。 | 显示「验收通过」，可打开交付包目录。 |
| WIN-02 | 把示例工单和 WIN-01 的 ZIP 拖入窗口，点击「打开验收报告」。 | 文件齐备后自动验收，生成独立结果和 ZIP 副本，HTML 报告由系统关联程序打开。 |
| WIN-03 | 重复 WIN-01，打包命令省略最后的 `migrations`。 | 显示「验收拒收」，报告列出缺失的 `migrations/1.6_to_1.7.sql`，可打开证据目录。 |
| WIN-04 | 把 WIN-03 的失败目录拖入窗口，点击「复跑此证据」。 | 切换到「复跑」并填入目录，按原命令生成新的拒收记录。复跑前后核对原工单、`execution.json`、迁移脚本快照和原 ZIP 的 SHA-256。 |

每次记录复核时间（含时区）、复核者、源码提交、ShipCheck 版本、Windows 版本及内部版本号、架构、Go/PowerShell 版本、GUI 程序 SHA-256 和测试输入来源。可用 `Get-FileHash -LiteralPath .\ShipCheck.exe -Algorithm SHA256` 计算 GUI 程序摘要。

| 编号 | 实际现象 | 结果（通过 / 失败 / 未执行） | 公开证据 |
| --- | --- | --- | --- |
| WIN-01 | 待填写 | 未执行 | 待补 |
| WIN-02 | 待填写 | 未执行 | 待补 |
| WIN-03 | 待填写 | 未执行 | 待补 |
| WIN-04 | 待填写 | 未执行 | 待补 |

## 保存公开证据

上面的历史摘要可直接公开。后续运行可将脱敏摘要存为 `docs/evidence/<UTC日期>-<短提交SHA>.md`，先填写复核时间、完整提交 SHA、环境版本和示例输入来源，再逐项填写以下字段。

| 验证项 | 公开摘要字段 |
| --- | --- |
| Go 测试和 `go vet` | 命令、实际退出码、各组顶层测试通过/失败/跳过数。 |
| 构建和 CLI 集成 | 构建退出码、CLI/GUI 二进制及公开示例 ZIP 的 SHA-256、脚本最终标记。每个场景保留实际退出码、总体状态、问题代码及公开示例中的相对路径。用案例编号关联原失败、复跑和修正。 |
| Windows 窗口 | 脚本最终标记，以及每项人工检查的实际现象、结果和脱敏证据。 |

附件发布后再在本页添加链接，并用未登录会话确认可访问。

从本次生成的 `receipt.json`、`verification.json` 和窗口记录提取所需字段。绝对路径、用户名、主机名、内部地址、命令参数中的敏感值、私有工单或 ZIP 内容应从公开摘要移除。原始 HTML 含路径和指向同目录日志、输入及 ZIP 的链接，公开时优先使用独立 Markdown 摘要。需要公开 HTML 或截图时，使用仓库示例重新生成并检查正文、链接和画面。

验收覆盖文件、组件声明、依赖条件和迁移顺序。迁移的执行效果、程序启动和运行行为需在目标环境另行试装验证。
