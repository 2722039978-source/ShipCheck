//go:build shipcheck_gui

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	d "github.com/lxn/walk/declarative"
)

func (app *shipCheckWindow) historyTab() d.TabPage {
	return d.TabPage{
		Title:  "历史记录",
		Layout: d.VBox{Spacing: 6},
		Children: []d.Widget{
			textRow("查询工单号", "", &app.historyOrder),
			textRow("查询版本", "", &app.historyVersion),
			textRow("状态筛选", "", &app.historyStatus),
			textRow("开始日期", "", &app.historyFrom),
			textRow("结束日期", "", &app.historyTo),
			d.Label{Text: "状态填写 passed 或 rejected，日期使用 YYYY-MM-DD；空白表示全部。"},
			actionRow("查询历史记录", app.refreshHistory),
			d.ListBox{AssignTo: &app.historyList, Model: []string{}, MinSize: d.Size{Height: 140}, StretchFactor: 1, OnCurrentIndexChanged: app.selectHistory},
		},
	}
}

func (app *shipCheckWindow) refreshHistory() {
	filter := historyFilter{
		OrderID:  strings.TrimSpace(app.historyOrder.Text()),
		Version:  strings.TrimSpace(app.historyVersion.Text()),
		Status:   strings.TrimSpace(app.historyStatus.Text()),
		FromDate: strings.TrimSpace(app.historyFrom.Text()),
		ToDate:   strings.TrimSpace(app.historyTo.Text()),
	}
	app.startJob("正在查询历史记录", func() (windowResult, error) {
		entries, err := listHistory(app.resultsRoot, filter)
		if err != nil {
			return windowResult{}, err
		}
		labels := make([]string, 0, len(entries))
		for _, entry := range entries {
			labels = append(labels, fmt.Sprintf("%s | %s | %s → %s | %s | %s", entry.CreatedAt, entry.OrderID, entry.FromVersion, entry.ToVersion, entry.Kind, entry.Status))
		}
		var modelErr error
		app.main.Synchronize(func() {
			app.historyEntries = entries
			modelErr = app.historyList.SetModel(labels)
		})
		if modelErr != nil {
			return windowResult{}, modelErr
		}
		return windowResult{status: "历史查询完成", details: fmt.Sprintf("共 %d 条记录\n选择记录后可以打开报告或结果目录。", len(entries))}, nil
	})
}

func (app *shipCheckWindow) selectHistory() {
	index := app.historyList.CurrentIndex()
	if index < 0 || index >= len(app.historyEntries) {
		return
	}
	entry := app.historyEntries[index]
	view := windowResult{
		status:    "历史记录",
		details:   fmt.Sprintf("工单 %s\n升级 %s → %s\n操作 %s\n结果 %s\n时间 %s\nZIP %s\n结果目录 %s", entry.OrderID, entry.FromVersion, entry.ToVersion, entry.Kind, entry.Status, entry.CreatedAt, entry.ZIPPath, entry.Directory),
		directory: entry.Directory,
		openLabel: "打开结果目录",
	}
	if entry.TimeNotice != "" {
		view.details += "\n" + entry.TimeNotice
	}
	path := filepath.Join(entry.Directory, "report.html")
	if reportFileExists(path) {
		view.reportPath = path
	}
	if entry.Status == statusRejected && isEvidenceDirectory(entry.Directory) {
		view.evidence = entry.Directory
	}
	app.showResult(view)
}
