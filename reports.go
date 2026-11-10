package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
)

func writeHTMLReport(path string, receipt receiptRecord) error {
	pretty := func(value any) (string, error) {
		data, err := json.MarshalIndent(value, "", "  ")
		return string(data), err
	}
	artifactURL := func(path string) string { return "artifact/" + url.PathEscape(filepath.Base(path)) }
	report, err := template.New("report").Funcs(template.FuncMap{"json": pretty, "artifactURL": artifactURL}).Parse(reportHTML)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return errors.Join(report.Execute(file, receipt), file.Close())
}

const reportHTML = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>ShipCheck 交付报告 · {{.Order.OrderID}}</title>
<style>
body{font-family:system-ui,"Microsoft YaHei",sans-serif;margin:0;background:#f5f7fa;color:#152638;line-height:1.65}main{max-width:1120px;margin:32px auto;padding:28px;background:white;border:1px solid #d5dfe7;border-radius:12px}h1,h2,h3{line-height:1.3}h2{margin-top:32px;border-bottom:1px solid #d5dfe7;padding-bottom:10px}table{width:100%;border-collapse:collapse;margin:12px 0}th,td{border:1px solid #d5dfe7;text-align:left;padding:10px;vertical-align:top;overflow-wrap:anywhere}th{background:#f0f4f8}pre{white-space:pre-wrap;overflow-wrap:anywhere;padding:16px;background:#f0f4f8;border-radius:6px}.passed{color:#176c44}.rejected{color:#a62828}.notice{color:#5b4c29;background:#fffae7;padding:12px}code{overflow-wrap:anywhere}a{color:#165bb0}.links{display:flex;gap:20px;flex-wrap:wrap}@media print{body{background:white}main{margin:0;border:0}h2{break-after:avoid}tr{break-inside:avoid}}
</style></head><body><main>
<h1>交付报告 · {{.Order.OrderID}}</h1>
<p class="{{.Status}}"><strong>总体状态：{{.Status}}</strong> · 操作：{{.Kind}}</p>
<table><tbody>
<tr><th>版本</th><td>{{.Order.FromVersion}} → {{.Order.ToVersion}}</td><th>平台</th><td>{{.Order.Platform}}</td></tr>
<tr><th>创建时间</th><td>{{.CreatedAt}}</td><th>ShipCheck 版本</th><td>{{.ToolVersion}}</td></tr>
<tr><th>ZIP 路径</th><td colspan="3">{{if .ZIPPath}}{{.ZIPPath}}{{else}}没有取得交付包{{end}}</td></tr>
<tr><th>ZIP SHA-256</th><td colspan="3"><code>{{.ZIPSHA256}}</code></td></tr>
{{if .SourceDirectory}}<tr><th>来源记录</th><td colspan="3">{{.SourceDirectory}}</td></tr>{{end}}
{{if .SourceZIPPath}}<tr><th>来源 ZIP</th><td colspan="3">{{.SourceZIPPath}}</td></tr>{{end}}
</tbody></table>
<p class="links"><a href="order.json">完整工单</a><a href="verification.json">验收 JSON</a><a href="receipt.json">记录 JSON</a>{{if .ZIPPath}}<a href="{{artifactURL .ZIPPath}}">保存的 ZIP</a>{{end}}{{if .Execution}}<a href="execution.json">命令记录</a><a href="stdout.log">标准输出</a><a href="stderr.log">错误输出</a><a href="input/">输入快照</a>{{end}}</p>
<h2>验收结果</h2><p>ZIP 验收状态：{{.Verification.Status}}；ZIP 可读取：{{.Verification.ArchiveReadable}}</p>
{{template "issues" .Verification.Issues}}
<h2>检查范围</h2>
<table><thead><tr><th>检查项</th><th>状态</th><th>范围</th></tr></thead><tbody>{{range .Verification.Checks}}<tr><td>{{.Name}}</td><td>{{.Status}}</td><td>{{.Scope}}</td></tr>{{end}}</tbody></table>
<p class="notice">报告记录文件内容、工单规则、组件声明、依赖条件与迁移顺序检查结果。目标环境负责试装和运行验证。</p>
<h2>文件摘要</h2><table><thead><tr><th>路径</th><th>字节数</th><th>SHA-256</th><th>可读取</th></tr></thead><tbody>{{range .Verification.Files}}<tr><td>{{.Path}}</td><td>{{.Size}}</td><td><code>{{.SHA256}}</code></td><td>{{.Readable}}</td></tr>{{end}}</tbody></table>
{{if .Execution}}<h2>打包执行</h2><pre>{{json .Execution}}</pre>{{end}}
{{if .Comparison}}<h2>问题变化</h2>
<h3>已解决问题</h3>{{template "issues" .Comparison.Resolved}}
<h3>持续存在问题</h3>{{template "issues" .Comparison.Persisting}}
<h3>新增问题</h3>{{template "issues" .Comparison.Added}}
<h3>无法确认的问题</h3>{{template "issues" .Comparison.Unverified}}
<h2>输入内容变化</h2>{{template "changes" .Comparison.InputChanges}}
<h2>ZIP 内容变化</h2>{{if .Comparison.ZIPComparisonAvailable}}{{template "changes" .Comparison.ZIPChanges}}{{else}}<p class="notice">{{.Comparison.ZIPComparisonNotice}}</p>{{end}}
{{end}}
<h2>完整工单规则</h2><pre>{{json .Order}}</pre>
</main></body></html>
{{define "issues"}}{{if .}}<table><thead><tr><th>问题</th><th>路径与字段</th><th>预期与实际</th><th>说明</th></tr></thead><tbody>{{range .}}<tr><td>{{.Code}}</td><td>{{.Path}}<br>{{.Field}}</td><td>预期：{{.Expected}}<br>实际：{{.Actual}}</td><td>{{.Message}}{{if .Advice}}<br>{{.Advice}}{{end}}</td></tr>{{end}}</tbody></table>{{else}}<p>没有记录问题。</p>{{end}}{{end}}
{{define "changes"}}{{if .}}<table><thead><tr><th>路径</th><th>变化</th><th>原内容</th><th>本次内容</th></tr></thead><tbody>{{range .}}<tr><td>{{.Path}}</td><td>{{.Change}}</td><td><pre>{{json .Before}}</pre></td><td><pre>{{json .After}}</pre></td></tr>{{end}}</tbody></table>{{else}}<p>内容没有变化。</p>{{end}}{{end}}`
