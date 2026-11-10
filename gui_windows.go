//go:build shipcheck_gui

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

const (
	verifyPage = iota
	runPage
	replayPage
	orderPage
	historyPage
)

const (
	orderFilter = "JSON 工单 (*.json)|*.json"
	zipFilter   = "ZIP 交付包 (*.zip)|*.zip"
)

type windowResult struct {
	status     string
	details    string
	directory  string
	openLabel  string
	evidence   string
	reportPath string
}

type shipCheckWindow struct {
	main            *walk.MainWindow
	tabs            *walk.TabWidget
	dropTarget      *walk.Label
	verifyOrder     *walk.LineEdit
	verifyZIP       *walk.LineEdit
	runOrder        *walk.LineEdit
	runStaging      *walk.LineEdit
	runOutput       *walk.LineEdit
	runCommand      *walk.LineEdit
	commandState    *packagingCommandState
	replayPath      *walk.LineEdit
	reviseStaging   *walk.LineEdit
	reviseCommand   *walk.LineEdit
	orderID         *walk.LineEdit
	orderFrom       *walk.LineEdit
	orderTo         *walk.LineEdit
	orderPlatform   *walk.LineEdit
	orderJSON       *walk.TextEdit
	templateName    *walk.LineEdit
	templateVersion *walk.LineEdit
	historyOrder    *walk.LineEdit
	historyVersion  *walk.LineEdit
	historyStatus   *walk.LineEdit
	historyFrom     *walk.LineEdit
	historyTo       *walk.LineEdit
	historyList     *walk.ListBox
	historyEntries  []historyEntry
	status          *walk.Label
	report          *walk.TextEdit
	openButton      *walk.PushButton
	replayButton    *walk.PushButton
	reportButton    *walk.PushButton
	resultsRoot     string
	busy            bool
	result          windowResult
}

func main() {
	runtime.LockOSThread()
	executable, err := os.Executable()
	if err != nil {
		walk.MsgBox(nil, "ShipCheck", err.Error(), walk.MsgBoxIconError)
		os.Exit(2)
	}
	app := shipCheckWindow{resultsRoot: filepath.Join(filepath.Dir(executable), ".shipcheck")}
	if err := app.create(); err != nil {
		walk.MsgBox(nil, "ShipCheck", err.Error(), walk.MsgBoxIconError)
		os.Exit(2)
	}
	os.Exit(app.main.Run())
}

func configurePackagingProcess(process *exec.Cmd) {
	process.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

func (app *shipCheckWindow) create() error {
	window := d.MainWindow{
		AssignTo:    &app.main,
		Title:       "ShipCheck",
		Size:        d.Size{Width: 960, Height: 850},
		MinSize:     d.Size{Width: 820, Height: 760},
		Layout:      d.VBox{Margins: d.Margins{Left: 16, Top: 14, Right: 16, Bottom: 14}, Spacing: 8},
		OnDropFiles: app.acceptDrop,
		Children: []d.Widget{
			d.Label{Text: "离线升级包检查", Font: d.Font{PointSize: 14, Bold: true}},
			d.Label{
				AssignTo: &app.dropTarget,
				MinSize:  d.Size{Height: 28},
				Text:     "将 JSON 工单和 ZIP 拖到这里，也可以拖入失败证据目录。",
			},
			d.TabWidget{
				AssignTo: &app.tabs,
				Pages:    []d.TabPage{app.verifyTab(), app.runTab(), app.replayTab(), app.orderTab(), app.historyTab()},
			},
			d.Label{AssignTo: &app.status, Font: d.Font{PointSize: 11, Bold: true}},
			d.TextEdit{AssignTo: &app.report, ReadOnly: true, VScroll: true, MinSize: d.Size{Height: 175}, StretchFactor: 1},
			d.Composite{
				Layout: d.HBox{MarginsZero: true},
				Children: []d.Widget{
					d.PushButton{AssignTo: &app.openButton, Text: "打开结果目录", OnClicked: app.openResultDirectory},
					d.PushButton{AssignTo: &app.replayButton, Text: "转到复跑", OnClicked: func() { app.selectEvidence(app.result.evidence) }},
					d.PushButton{AssignTo: &app.reportButton, Text: "打开验收报告", OnClicked: func() { app.openPath(app.result.reportPath) }},
					d.HSpacer{},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		return err
	}
	app.dropTarget.DropFiles().Attach(app.acceptDrop)
	app.main.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if app.busy {
			*canceled = true
		}
	})
	app.main.Disposing().Attach(func() { walk.App().Exit(0) })
	app.showResult(windowResult{
		status:  "等待文件",
		details: "选择或拖入 JSON 工单和 ZIP，文件齐备后自动验收。",
	})
	return nil
}

func (app *shipCheckWindow) verifyTab() d.TabPage {
	return d.TabPage{
		Title:  "验收 ZIP",
		Layout: d.VBox{Spacing: 8},
		Children: []d.Widget{
			pathRow("工单 JSON", "选择工单", &app.verifyOrder, func() {
				app.choosePath(app.verifyOrder, "选择工单", orderFilter, app.verifyWhenReady)
			}),
			pathRow("交付 ZIP", "选择 ZIP", &app.verifyZIP, func() {
				app.choosePath(app.verifyZIP, "选择 ZIP", zipFilter, app.verifyWhenReady)
			}),
			actionRow("重新验收", app.verifySelected),
			d.VSpacer{},
		},
	}
}

func (app *shipCheckWindow) runTab() d.TabPage {
	return d.TabPage{
		Title:  "打包",
		Layout: d.VBox{Spacing: 8},
		Children: []d.Widget{
			pathRow("工单 JSON", "选择打包工单", &app.runOrder, func() {
				app.choosePath(app.runOrder, "选择打包工单", orderFilter, nil)
			}),
			pathRow("暂存目录", "选择暂存目录", &app.runStaging, func() {
				app.choosePath(app.runStaging, "选择暂存目录", "", nil)
			}),
			textRow("ZIP 输出路径", "upgrade.zip", &app.runOutput),
			textRow("打包命令", "", &app.runCommand),
			actionRow("打包并验收", app.runSelected),
			d.VSpacer{},
		},
	}
}

func (app *shipCheckWindow) replayTab() d.TabPage {
	return d.TabPage{
		Title:  "复跑",
		Layout: d.VBox{Spacing: 8},
		Children: []d.Widget{
			pathRow("证据目录", "选择证据目录", &app.replayPath, func() {
				app.choosePath(app.replayPath, "选择证据目录", "", nil)
			}),
			actionRow("复跑此证据", app.replaySelected),
			d.Label{Text: "创建修正尝试：选择修正后的输入目录或填写新命令。空白项目使用原记录。"},
			pathRow("修正输入目录", "选择修正输入", &app.reviseStaging, func() {
				app.choosePath(app.reviseStaging, "选择修正后的输入目录", "", nil)
			}),
			textRow("修正打包命令", "", &app.reviseCommand),
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.PushButton{Text: "创建修正尝试", OnClicked: app.reviseSelected},
				d.PushButton{Text: "清空修正内容", OnClicked: func() { app.reviseStaging.SetText(""); app.reviseCommand.SetText("") }},
				d.HSpacer{},
			}},
			d.VSpacer{},
		},
	}
}

func pathRow(label, button string, path **walk.LineEdit, onChoose func()) d.Composite {
	return d.Composite{
		Layout: d.HBox{MarginsZero: true},
		Children: []d.Widget{
			d.Label{Text: label, MinSize: d.Size{Width: 95}},
			d.LineEdit{AssignTo: path, ReadOnly: true, StretchFactor: 1},
			d.PushButton{Text: button, MinSize: d.Size{Width: 105}, OnClicked: onChoose},
		},
	}
}

func textRow(label, initial string, value **walk.LineEdit) d.Composite {
	return d.Composite{
		Layout: d.HBox{MarginsZero: true},
		Children: []d.Widget{
			d.Label{Text: label, MinSize: d.Size{Width: 95}},
			d.LineEdit{AssignTo: value, Text: initial, StretchFactor: 1},
		},
	}
}

func actionRow(text string, onClick func()) d.Composite {
	return d.Composite{
		Layout: d.HBox{MarginsZero: true},
		Children: []d.Widget{
			d.PushButton{Text: text, MinSize: d.Size{Width: 105}, OnClicked: onClick},
			d.HSpacer{},
		},
	}
}

func (app *shipCheckWindow) choosePath(field *walk.LineEdit, title, filter string, after func()) {
	dialog := walk.FileDialog{Title: title, Filter: filter, FilePath: field.Text()}
	var accepted bool
	var err error
	if filter == "" {
		accepted, err = dialog.ShowBrowseFolder(app.main)
	} else {
		accepted, err = dialog.ShowOpen(app.main)
	}
	if err != nil {
		app.showError(err)
		return
	}
	if accepted {
		field.SetText(dialog.FilePath)
		if after != nil {
			after()
		}
	}
}

func (app *shipCheckWindow) acceptDrop(paths []string) {
	if app.busy || len(paths) == 0 {
		return
	}
	page := app.tabs.CurrentIndex()
	if len(paths) == 1 {
		path := paths[0]
		info, err := os.Stat(path)
		if err != nil {
			app.showError(err)
			return
		}
		if info.IsDir() {
			switch {
			case isEvidenceDirectory(path) || page == replayPage:
				app.selectEvidence(path)
			case page == runPage:
				app.runStaging.SetText(path)
			default:
				app.showError(errors.New("请在打包页面拖入暂存目录，或拖入包含失败记录的证据目录"))
			}
			return
		}
		if page == runPage && strings.EqualFold(filepath.Ext(path), ".json") {
			app.runOrder.SetText(path)
			return
		}
	}

	var orderPath, zipPath string
	for _, path := range paths {
		switch {
		case orderPath == "" && strings.EqualFold(filepath.Ext(path), ".json"):
			orderPath = path
		case zipPath == "" && strings.EqualFold(filepath.Ext(path), ".zip"):
			zipPath = path
		default:
			app.showError(errors.New("一次请选择一份 JSON 工单和一个 ZIP"))
			return
		}
	}
	if err := app.tabs.SetCurrentIndex(verifyPage); err != nil {
		app.showError(err)
		return
	}
	if orderPath != "" {
		app.verifyOrder.SetText(orderPath)
	}
	if zipPath != "" {
		app.verifyZIP.SetText(zipPath)
	}
	app.verifyWhenReady()
}

func isEvidenceDirectory(path string) bool {
	record, recordErr := os.Stat(filepath.Join(path, executionFilename))
	input, inputErr := os.Stat(filepath.Join(path, inputDirectory))
	return recordErr == nil && record.Mode().IsRegular() && inputErr == nil && input.IsDir()
}

func (app *shipCheckWindow) selectEvidence(path string) {
	if path == "" {
		return
	}
	if err := app.tabs.SetCurrentIndex(replayPage); err != nil {
		app.showError(err)
		return
	}
	app.replayPath.SetText(path)
	app.reviseStaging.SetText("")
	app.reviseCommand.SetText("")
}

func (app *shipCheckWindow) verifyWhenReady() {
	if app.verifyOrder.Text() == "" || app.verifyZIP.Text() == "" {
		app.showResult(windowResult{status: "等待文件", details: "工单和 ZIP 齐备后自动验收。"})
		return
	}
	app.verifySelected()
}

func (app *shipCheckWindow) verifySelected() {
	orderPath, zipPath := app.verifyOrder.Text(), app.verifyZIP.Text()
	if orderPath == "" || zipPath == "" {
		app.showError(errors.New("请选择 JSON 工单和交付 ZIP"))
		return
	}
	app.startJob("正在验收 ZIP", func() (windowResult, error) {
		saved, err := verifyAndRecord(orderPath, zipPath, app.resultsRoot)
		if err != nil {
			return windowResult{}, err
		}
		var details strings.Builder
		report := saved.Report
		report.writeTo(&details)
		fmt.Fprintln(&details, "验收结果目录", saved.Directory)
		view := windowResult{status: "验收拒收", details: details.String(), directory: saved.Directory, openLabel: "打开验收结果目录", reportPath: filepath.Join(saved.Directory, "report.html")}
		if report.Status == statusPassed {
			view.status = "验收通过"
			view.directory = filepath.Dir(report.ZIPPath)
			view.openLabel = "打开 ZIP 所在目录"
		}
		return view, nil
	})
}

func (app *shipCheckWindow) runSelected() {
	orderPath, staging, output := app.runOrder.Text(), app.runStaging.Text(), app.runOutput.Text()
	commandLine := app.runCommand.Text()
	if orderPath == "" || staging == "" || output == "" || strings.TrimSpace(commandLine) == "" {
		app.showError(errors.New("请选择工单和暂存目录，并填写 ZIP 输出路径与打包命令"))
		return
	}
	command, err := app.packagingCommand(commandLine)
	if err != nil {
		app.showError(err)
		return
	}
	app.startJob("正在打包并验收", func() (windowResult, error) {
		result, err := runPackaging(orderPath, staging, output, app.resultsRoot, command)
		if err != nil {
			return windowResult{}, err
		}
		return attemptView(result, nil), nil
	})
}

func (app *shipCheckWindow) replaySelected() {
	path := app.replayPath.Text()
	if path == "" {
		app.showError(errors.New("请选择失败的证据目录"))
		return
	}
	app.startJob("正在复跑并验收", func() (windowResult, error) {
		result, original, err := replayPackaging(path, app.resultsRoot)
		if err != nil {
			return windowResult{}, err
		}
		return attemptView(result, &original), nil
	})
}

func attemptView(result attempt, original *verification) windowResult {
	var details strings.Builder
	result.writeTo(&details)
	view := windowResult{status: "验收拒收", directory: result.Directory, openLabel: "打开结果目录", reportPath: filepath.Join(result.Directory, "report.html")}
	switch {
	case result.passed():
		view.status = "验收通过"
		view.directory = filepath.Dir(result.Verification.ZIPPath)
		view.openLabel = "打开交付包目录"
	case !result.Execution.passed():
		view.status = "打包命令失败"
	}
	if original != nil {
		writeMissingComparison(&details, *original, result.Verification)
		fmt.Fprintln(&details, "复跑结果目录", result.Directory)
	}
	if !result.passed() {
		view.evidence = result.Directory
		view.openLabel = "打开证据目录"
		fmt.Fprintln(&details, "证据目录", result.Directory)
	}
	view.details = details.String()
	return view
}

func (app *shipCheckWindow) startJob(status string, work func() (windowResult, error)) {
	if app.busy {
		return
	}
	app.busy = true
	app.tabs.SetEnabled(false)
	app.showResult(windowResult{status: status, details: "正在处理，请等待本次操作完成。"})
	go func() {
		result, err := work()
		app.main.Synchronize(func() {
			app.busy = false
			app.tabs.SetEnabled(true)
			if err != nil {
				app.showError(err)
				return
			}
			app.showResult(result)
		})
	}()
}

func (app *shipCheckWindow) showError(err error) {
	app.showResult(windowResult{status: "操作失败", details: err.Error()})
}

func (app *shipCheckWindow) showResult(result windowResult) {
	app.result = result
	app.status.SetText(result.status)
	details := strings.ReplaceAll(result.details, "\r\n", "\n")
	app.report.SetText(strings.ReplaceAll(details, "\n", "\r\n"))
	label := result.openLabel
	if label == "" {
		label = "打开结果目录"
	}
	app.openButton.SetText(label)
	app.openButton.SetEnabled(result.directory != "")
	app.replayButton.SetEnabled(result.evidence != "")
	app.reportButton.SetEnabled(result.reportPath != "")
}

func (app *shipCheckWindow) openResultDirectory() {
	app.openPath(app.result.directory)
}

func (app *shipCheckWindow) openPath(path string) {
	if path == "" {
		return
	}
	name, err := windows.UTF16PtrFromString(path)
	if err == nil {
		err = windows.ShellExecute(windows.Handle(app.main.Handle()), nil, name, nil, nil, windows.SW_SHOWNORMAL)
	}
	if err != nil {
		walk.MsgBox(app.main, "打开目录失败", err.Error(), walk.MsgBoxIconError)
	}
}
