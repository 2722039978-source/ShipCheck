//go:build shipcheck_gui

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

type packagingCommandState struct {
	command commandSpec
	text    string
}

func (app *shipCheckWindow) loadPackagingCommand(command commandSpec) string {
	command.Args = slices.Clone(command.Args)
	tool := command.Tool
	if command.WorkspaceTool != "" {
		tool = ".\\" + filepath.FromSlash(command.WorkspaceTool)
	} else if command.Executable != "" {
		tool = command.Executable
	}
	text := ""
	if tool != "" {
		text = windows.ComposeCommandLine(append([]string{tool}, command.Args...))
	}
	app.commandState = &packagingCommandState{command: command, text: text}
	return text
}

func (app *shipCheckWindow) packagingCommand(commandLine string) (commandSpec, error) {
	if app.commandState != nil && commandLine == app.commandState.text {
		command := app.commandState.command
		command.Args = slices.Clone(command.Args)
		return command, nil
	}
	commandLine = strings.TrimSpace(commandLine)
	if commandLine == "" {
		return commandSpec{}, nil
	}
	args, err := windows.DecomposeCommandLine(commandLine)
	if err != nil {
		return commandSpec{}, fmt.Errorf("读取打包命令失败 %w", err)
	}
	if len(args) == 0 || args[0] == "" {
		return commandSpec{}, fmt.Errorf("请输入打包工具的名称或路径")
	}
	return commandSpec{Tool: args[0], Args: append([]string{}, args[1:]...)}, nil
}

func (app *shipCheckWindow) orderTab() d.TabPage {
	return d.TabPage{
		Title:  "工单与模板",
		Layout: d.VBox{Spacing: 6},
		Children: []d.Widget{
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.PushButton{Text: "新建工单", OnClicked: app.newOrder},
				d.PushButton{Text: "读取工单", OnClicked: app.readOrderEditor},
				d.PushButton{Text: "读取模板", OnClicked: app.readTemplateEditor},
				d.HSpacer{},
			}},
			textRow("编辑工单号", "", &app.orderID),
			textRow("起始版本", "", &app.orderFrom),
			textRow("目标版本", "", &app.orderTo),
			textRow("目标平台", "windows-amd64", &app.orderPlatform),
			d.Label{Text: "完整工单 JSON（文件摘要、目录规则、组件与迁移）。保存时使用上方基本信息。"},
			d.TextEdit{AssignTo: &app.orderJSON, VScroll: true, MinSize: d.Size{Height: 100}, StretchFactor: 1},
			textRow("模板名称", "", &app.templateName),
			textRow("模板版本", "", &app.templateVersion),
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.PushButton{Text: "校验工单", OnClicked: app.validateEditor},
				d.PushButton{Text: "保存工单并使用", OnClicked: app.saveEditorOrder},
				d.PushButton{Text: "保存为模板", OnClicked: app.saveEditorTemplate},
				d.HSpacer{},
			}},
		},
	}
}

func (app *shipCheckWindow) setEditorOrder(ticket order) error {
	data, err := json.MarshalIndent(ticket, "", "  ")
	if err != nil {
		return err
	}
	app.orderID.SetText(ticket.OrderID)
	app.orderFrom.SetText(ticket.FromVersion)
	app.orderTo.SetText(ticket.ToVersion)
	app.orderPlatform.SetText(ticket.Platform)
	return app.orderJSON.SetText(strings.ReplaceAll(string(data), "\n", "\r\n"))
}

func (app *shipCheckWindow) newOrder() {
	ticket := order{RequiredFiles: []string{}, releaseMetadata: releaseMetadata{Platform: "windows-amd64"}}
	if err := app.setEditorOrder(ticket); err != nil {
		app.showError(err)
		return
	}
	app.templateName.SetText("")
	app.templateVersion.SetText("")
	app.showResult(windowResult{status: "填写新工单", details: "填写工单号、版本及所需文件规则，完成后点击校验工单。文件预期摘要应来自已经确认的发布清单。"})
}

func (app *shipCheckWindow) editedOrder() (order, error) {
	ticket, err := decodeTemplateJSON[order]([]byte(app.orderJSON.Text()))
	if err != nil {
		return ticket, fmt.Errorf("读取工单 JSON 失败 %w", err)
	}
	ticket.OrderID = strings.TrimSpace(app.orderID.Text())
	ticket.FromVersion = strings.TrimSpace(app.orderFrom.Text())
	ticket.ToVersion = strings.TrimSpace(app.orderTo.Text())
	ticket.Platform = strings.TrimSpace(app.orderPlatform.Text())
	if err := validateOrder(ticket); err != nil {
		return ticket, err
	}
	return ticket, nil
}

func (app *shipCheckWindow) validateEditor() {
	ticket, err := app.editedOrder()
	if err != nil {
		app.showError(err)
		return
	}
	if err := app.setEditorOrder(ticket); err != nil {
		app.showError(err)
		return
	}
	app.showResult(windowResult{status: "工单有效", details: fmt.Sprintf("工单 %s\n升级 %s → %s\n平台 %s\n可以保存工单并用于验收或打包。", ticket.OrderID, ticket.FromVersion, ticket.ToVersion, ticket.Platform)})
}

func (app *shipCheckWindow) chooseJSONFile(title string, save bool, action func(string) error) {
	dialog := walk.FileDialog{Title: title, Filter: orderFilter}
	var accepted bool
	var err error
	if save {
		dialog.FilePath = "delivery.json"
		accepted, err = dialog.ShowSave(app.main)
	} else {
		accepted, err = dialog.ShowOpen(app.main)
	}
	if err != nil {
		app.showError(err)
		return
	}
	if accepted {
		if save && filepath.Ext(dialog.FilePath) == "" {
			dialog.FilePath += ".json"
		}
		if err := action(dialog.FilePath); err != nil {
			app.showError(err)
		}
	}
}

func (app *shipCheckWindow) readOrderEditor() {
	app.chooseJSONFile("读取交付工单", false, func(path string) error {
		ticket, _, err := loadOrder(path)
		if err != nil {
			return err
		}
		if err := app.setEditorOrder(ticket); err != nil {
			return err
		}
		app.showResult(windowResult{status: "工单已读取", details: path})
		return nil
	})
}

func (app *shipCheckWindow) readTemplateEditor() {
	app.chooseJSONFile("读取工单模板", false, func(path string) error {
		value, err := loadTemplate(path)
		if err != nil {
			return err
		}
		ticket, err := instantiateTemplate(value, value.Order.OrderID, value.Order.FromVersion, value.Order.ToVersion)
		if err != nil {
			return err
		}
		if err := app.setEditorOrder(ticket); err != nil {
			return err
		}
		app.templateName.SetText(value.Name)
		app.templateVersion.SetText(value.Version)
		app.runOutput.SetText(value.Output)
		if err := app.runCommand.SetText(app.loadPackagingCommand(value.Command)); err != nil {
			return err
		}
		app.showResult(windowResult{status: "模板已读取", details: fmt.Sprintf("模板 %s\n版本 %s\n请填写本次工单号并确认完整规则。打包页面已载入模板默认值。", value.Name, value.Version)})
		return nil
	})
}

func (app *shipCheckWindow) saveEditorOrder() {
	ticket, err := app.editedOrder()
	if err != nil {
		app.showError(err)
		return
	}
	app.chooseJSONFile("将工单保存到新文件", true, func(path string) error {
		if err := saveOrder(path, ticket); err != nil {
			return err
		}
		app.verifyOrder.SetText(path)
		app.runOrder.SetText(path)
		if err := app.setEditorOrder(ticket); err != nil {
			return err
		}
		app.showResult(windowResult{status: "工单已保存", details: fmt.Sprintf("%s\n验收和打包页面已使用此工单。", path)})
		return nil
	})
}

func (app *shipCheckWindow) saveEditorTemplate() {
	ticket, err := app.editedOrder()
	if err != nil {
		app.showError(err)
		return
	}
	value := orderTemplate{Name: strings.TrimSpace(app.templateName.Text()), Version: strings.TrimSpace(app.templateVersion.Text()), Order: ticket, Output: app.runOutput.Text()}
	value.Command, err = app.packagingCommand(app.runCommand.Text())
	if err != nil {
		app.showError(err)
		return
	}
	app.chooseJSONFile("将模板保存到新文件", true, func(path string) error {
		if err := saveTemplate(path, value); err != nil {
			return err
		}
		app.showResult(windowResult{status: "模板已保存", details: fmt.Sprintf("%s\n模板包含完整工单规则以及打包页面的默认输出路径和命令。", path)})
		return nil
	})
}

func (app *shipCheckWindow) reviseSelected() {
	path := app.replayPath.Text()
	staging := app.reviseStaging.Text()
	commandLine := strings.TrimSpace(app.reviseCommand.Text())
	if path == "" || (staging == "" && commandLine == "") {
		app.showError(fmt.Errorf("请选择失败记录，并填写修正命令或选择修正输入"))
		return
	}
	var command *commandSpec
	if commandLine != "" {
		args, err := windows.DecomposeCommandLine(commandLine)
		if err != nil || len(args) == 0 || args[0] == "" {
			app.showError(fmt.Errorf("修正命令无效"))
			return
		}
		command = &commandSpec{Tool: args[0], Args: append([]string{}, args[1:]...)}
	}
	app.startJob("正在创建修正尝试", func() (windowResult, error) {
		result, original, err := revisePackaging(path, staging, app.resultsRoot, command)
		if err != nil {
			return windowResult{}, err
		}
		view := attemptView(result, &original)
		view.details += fmt.Sprintf("\n修正结果目录 %s\n", result.Directory)
		return view, nil
	})
}

func reportFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
