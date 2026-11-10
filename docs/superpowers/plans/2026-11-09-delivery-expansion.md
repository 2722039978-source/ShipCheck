# ShipCheck 交付检查扩展实施计划

采用 superpowers:subagent-driven-development 和真实文件测试执行，最终使用独立代码审查与完整验收。

目标：完成已批准的五项功能，提供可直接运行的 Windows 程序。

结构：Go 业务函数同时服务 CLI 和 Walk 窗口。验收规则、记录报告和工单模板分别维护，窗口只组织输入与结果。

使用的技术：Go 标准库、Walk、Windows 原生控件、PowerShell、tar.exe。

设计：[交付检查扩展](../specs/2026-11-09-delivery-expansion.md)。

## 全局要求

- 保留旧工单和旧失败记录的可用性；新增格式严格验证，无效输入就地返回错误。
- 仅使用文件编辑工具修改源码；测试不得使用 mock；不访问 /tmp。
- 新报告和修正记录不覆盖已有记录。构建缓存与测试文件位于 .local。
- 目录当前没有 Git 仓库，在用户指定目录内实施。

## 任务与接口

- [x] 验收规则：修改 verify.go，新增 rules.go 和 verify_test.go。保留 loadOrder、verifyZIP、verification 和 issue 的现有接口；新增 validateOrder(order) error 供模板与编辑器调用。使用标准 JSON 与 ZIP 库。测试正确摘要、错误摘要、长度、空内容、整个 ZIP 规则、组件依赖和迁移连续性。
- [x] 修正与报告：修改 workflow.go，新增 records.go、revision.go、reports.go 和对应测试。verifyAndRecord(orderPath, zipPath, resultsPath string) (savedVerification, error)，savedVerification 含 Directory string、Report verification。revisePackaging(evidencePath, stagingOverride, resultsPath string, commandOverride *commandSpec) (attempt, verification, error)。listHistory(resultsPath string, filter historyFilter) ([]historyEntry, error)，filter 含 OrderID、Version、Status、FromDate、ToDate string；entry 含 Directory、OrderID、FromVersion、ToVersion、Status、Kind、CreatedAt、ZIPPath string。每个结果目录生成 report.html 和 receipt.json。使用真实 tar.exe 验证修正、记录、报告及查询。
- [x] 工单模板：新增 templates.go 和 templates_test.go。模板类型 orderTemplate 含 Name、Version string、Order order、Output string、Command commandSpec。loadTemplate(path string) (orderTemplate, error)，saveTemplate(path string, value orderTemplate) error，instantiateTemplate(value orderTemplate, id, from, to string) (order, error)，saveOrder(path string, value order) error。模板来源保存在 order.Template *templateReference，含 Name、Version、SHA256 string。提供不依赖窗口的严格 JSON 编辑校验。验证模板来源、独立复制、无效规则和覆盖保护。
- [x] CLI 与窗口：修改 main.go、gui_windows.go，新增 cli_extended.go、gui_orders.go、gui_history.go。CLI 增加 revise、history、order、template，并给 verify 增加 results。窗口接入新业务函数。使用实际控件操作验证工单编辑、模板、修正和历史。
- [x] 文档与交付：更新 README.md、docs/implementation.md、examples，新增维护中的测试脚本。构建两种入口，执行 Go 测试、vet、原验收和新增验收，独立审查，再重新验证修复内容。

执行证据见 [验证记录](../../validation.md)。

## 审查重点

- 错误版本但同名文件被拒收；对应验收规则测试。
- 新输入、新命令与旧证据保持独立；对应修正集成测试。
- 命令失败但 ZIP 检查通过仍判定任务失败；对应报告与历史测试。
- 模板更新不能影响已经创建的工单；对应模板复制测试。
- 不完整迁移、组件冲突及新错误必须出现在报告中；对应规则与比较测试。
