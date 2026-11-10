# ShipCheck 0.2.0 验证记录

验证日期：2026-11-09。

## 程序

- 图形入口：`../ShipCheck.exe`。
- 命令行入口：`../.local/shipcheck.exe`。
- 图形程序 SHA-256：`BD6DC77104DF0FB3AA85608B6A285B2E51327DA56C59FCD5C244988E711BC9AE`。
- 命令行程序 SHA-256：`05FCE7F9BBB490716024D7BDEBCD5201E0A0A65EF013FDBD99AF2F1A8B0E0333`。

## 已执行检查

| 检查 | 结果 | 记录 |
| --- | --- | --- |
| Go 测试及 GUI 模板命令测试 | 40 个顶层测试通过，包含其全部子测试 | [测试与构建](../.local/validation-20261109.txt) |
| CLI 与 GUI 的 go vet | 通过 | [测试与构建](../.local/validation-20261109.txt) |
| CLI 与 GUI 构建 | 通过 | [测试与构建](../.local/validation-20261109.txt) |
| 新增 CLI 完整流程 | 通过 | [测试与构建](../.local/validation-20261109.txt) |
| 工单编辑、保存、模板读取、修正和历史选择 | 通过 | [窗口验证](../.local/gui-validation-20261109.txt) |
| 正常打包、独立验收、缺件拒收和原命令复跑 | 通过 | [流程回归](../.local/regression-20261109.txt) |
| 版本不符、输入缺件、命令失败 | 通过 | [失败场景](../.local/failure-validation-20261109.txt) |
| 拖放验收、窗口打包、缺件拒收、复跑和证据拖放 | 通过 | [窗口回归](../.local/gui-regression-20261109.txt) |
| gofmt 格式检查、行尾空白检查 | 通过 | 全部 Go 源文件及维护中的文档与脚本 |

测试使用真实 ZIP、文件、本机 tar.exe 与 Windows 控件。正常交付退出码为 0，拒收及打包失败退出码为 1。原失败工单、命令、输入和 ZIP 的内容保持原样。独立代码审查完成，复核未发现仍然阻碍交付的问题。

## 可查看结果

- [修正成功的 HTML 报告](../.local/extended-72759a167c414284885dd9b27e445423/results/revise-2022709989/report.html)。
- [同次修正的记录 JSON](../.local/extended-72759a167c414284885dd9b27e445423/results/revise-2022709989/receipt.json)。
- [窗口保存的工单](../.local/gui-extended-e7d17e59502b45a09da4376472392e88/gui-order.json)。
- [窗口保存的模板](../.local/gui-extended-e7d17e59502b45a09da4376472392e88/gui-template.json)。

文件检查、组件声明、依赖条件和迁移顺序属于本次验收范围。迁移执行效果和程序运行情况由目标环境试装验证。
