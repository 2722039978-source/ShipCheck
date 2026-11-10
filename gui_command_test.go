//go:build shipcheck_gui

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestGUICommandExecutableTemplate(t *testing.T) {
	template, err := loadTemplate(filepath.Join("examples", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := exec.LookPath("tar.exe")
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	template.Command.Tool = "delivery-archive-tool"
	template.Command.Executable = executable
	directory := rulesDirectory(t)
	inputPath := filepath.Join(directory, "input-template.json")
	if err := saveTemplate(inputPath, template); err != nil {
		t.Fatal(err)
	}
	template, err = loadTemplate(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	app := shipCheckWindow{}
	line := app.loadPackagingCommand(template.Command)
	args, err := windows.DecomposeCommandLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) == 0 || args[0] != executable {
		t.Errorf("显示的工具必须为模板执行文件：%q", line)
	}
	command, err := app.packagingCommand(line)
	if err != nil {
		t.Fatal(err)
	}
	orderPath := filepath.Join(directory, "order.json")
	if err := saveOrder(orderPath, template.Order); err != nil {
		t.Fatal(err)
	}
	result, err := runPackaging(orderPath, filepath.Join("examples", "staging"), template.Output, filepath.Join(directory, "results"), command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.passed() {
		t.Errorf("模板指定的 tar.exe 未完成打包与验收：命令 %+v，验收 %+v", result.Execution, result.Verification)
	}
	if result.Execution.Command.Tool != template.Command.Tool || result.Execution.Command.Executable != executable {
		t.Errorf("执行记录未保留模板指定的命令：%+v", result.Execution.Command)
	}
	template.Command = command
	templatePath := filepath.Join(directory, "template.json")
	if err := saveTemplate(templatePath, template); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadTemplate(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Command, result.Execution.Command) {
		t.Errorf("保存后的模板命令与执行命令不符：%+v", loaded.Command)
	}
}

func TestGUICommandWorkspacePriorityAndEditing(t *testing.T) {
	directory := rulesDirectory(t)
	original := commandSpec{
		Tool:          "delivery-archive-tool",
		Executable:    filepath.Join(directory, "external.exe"),
		WorkspaceTool: "tools/package tool.exe",
		Args:          []string{"--archive", "交付 包.zip", ""},
	}
	app := shipCheckWindow{}
	line := app.loadPackagingCommand(original)
	args, err := windows.DecomposeCommandLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) == 0 || args[0] != ".\\"+filepath.FromSlash(original.WorkspaceTool) {
		t.Errorf("工作目录工具未明确显示：%q", line)
	}
	command, err := app.packagingCommand(line)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(command, original) {
		t.Errorf("模板命令的完整字段没有保留：%+v", command)
	}
	resolved, err := command.resolve(directory)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Executable != filepath.Join(directory, filepath.FromSlash(original.WorkspaceTool)) {
		t.Errorf("工作目录工具的执行顺序不符：%+v", resolved)
	}
	original.Args[0] = "changed-source"
	command.Args[1] = "changed-result"
	retained, err := app.packagingCommand(line)
	if err != nil {
		t.Fatal(err)
	}
	if retained.Args[0] != "--archive" || retained.Args[1] != "交付 包.zip" {
		t.Fatalf("修改命令参数影响了保存的模板默认值：%+v", retained)
	}
	edited := windows.ComposeCommandLine([]string{"tar.exe", "-a", "-cf", "edited.zip", "release.json"})
	changed, err := app.packagingCommand(edited)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Tool != "tar.exe" || changed.Executable != "" || changed.WorkspaceTool != "" || !reflect.DeepEqual(changed.Args, []string{"-a", "-cf", "edited.zip", "release.json"}) {
		t.Fatalf("编辑命令后没有使用新内容：%+v", changed)
	}
	empty, err := app.packagingCommand(" ")
	if err != nil || empty.Tool != "" || empty.Executable != "" || empty.WorkspaceTool != "" || len(empty.Args) != 0 {
		t.Fatalf("清空命令后仍保留执行内容：%+v，错误 %v", empty, err)
	}
	if _, err := app.packagingCommand(`"" --archive`); err == nil {
		t.Fatal("空工具名称必须报告错误")
	}
}

func TestGUICommandWorkspaceTarExecution(t *testing.T) {
	directory := rulesDirectory(t)
	staging := filepath.Join(directory, "staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := exec.LookPath("tar.exe")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "package.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, releasePath), []byte(rulesRelease), 0600); err != nil {
		t.Fatal(err)
	}
	ticket := rulesOrder(t, map[string]any{"required_files": []string{}})
	orderPath := filepath.Join(directory, "order.json")
	if err := saveOrder(orderPath, ticket); err != nil {
		t.Fatal(err)
	}
	original := commandSpec{Tool: "delivery-archive-tool", Executable: executable, WorkspaceTool: "package.exe", Args: []string{"-a", "-cf", "delivery.zip", releasePath}}
	app := shipCheckWindow{}
	line := app.loadPackagingCommand(original)
	if !strings.HasPrefix(line, ".\\package.exe ") {
		t.Errorf("根目录工具未明确显示为相对路径：%q", line)
	}
	command, err := app.packagingCommand(line)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runPackaging(orderPath, staging, "delivery.zip", filepath.Join(directory, "results"), command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.passed() {
		t.Fatalf("工作目录内的真实 tar.exe 执行失败：%+v", result)
	}
	if result.Execution.Command.WorkspaceTool != "package.exe" || result.Execution.Command.Executable != filepath.Join(result.Execution.WorkingDir, "package.exe") || result.Execution.Command.Tool != original.Tool {
		t.Fatalf("工作目录工具的执行记录不符：%+v", result.Execution.Command)
	}
}
