package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func templateTestDirectory(t *testing.T) string {
	t.Helper()
	root := filepath.Join(".local", "template-tests")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "case-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOrderJSONRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(templateTestDirectory(t), "order.json")
	data := []byte(`{"order_id":"T-001","from_version":"1.0","to_version":"2.0","platform":"windows-x64","required_files":[],"unknown_rule":true}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadOrder(path); err == nil {
		t.Fatal("包含未知字段的工单必须拒绝")
	}
}

func templateFixture(t *testing.T) orderTemplate {
	t.Helper()
	data := []byte(`{
		"order_id":"TEMPLATE-001","from_version":"1.0","to_version":"2.0","platform":"windows-amd64",
		"required_files":["app/app.bin","migrations/upgrade.sql"],
		"files":[{"path":"app/app.bin","sha256":"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad","size":3,"allow_empty":false}],
		"allowed_directories":["app","migrations"],"forbidden_patterns":["**/*.bak"],"allow_extra_files":false,
		"components":[{"name":"base","version":"1.0"},{"name":"app","version":"2.0","dependencies":[{"name":"base","allowed_versions":["1.0"]}]}],
		"migrations":[{"id":"upgrade","from_version":"1.0","to_version":"2.0","files":["migrations/upgrade.sql"]}]
	}`)
	var ticket order
	if err := json.Unmarshal(data, &ticket); err != nil {
		t.Fatal(err)
	}
	return orderTemplate{Name: "产品发布", Version: "1", Order: ticket, Output: "upgrade.zip", Command: commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "upgrade.zip", "release.json", "app", "migrations"}}}
}

func TestTemplateRoundTripAndIndependentOrder(t *testing.T) {
	value := templateFixture(t)
	path := filepath.Join(templateTestDirectory(t), "Template.json")
	if err := saveTemplate(path, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadTemplate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(value, loaded) {
		t.Fatalf("模板读取后内容不符：%+v", loaded)
	}
	ticket, err := instantiateTemplate(loaded, "DELIVERY-002", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.OrderID != "DELIVERY-002" || ticket.FromVersion != "1.0" || ticket.ToVersion != "2.0" {
		t.Fatalf("工单基本信息不符：%+v", ticket)
	}
	if ticket.Template == nil || ticket.Template.Name != "产品发布" || ticket.Template.Version != "1" {
		t.Fatalf("工单缺少模板来源：%+v", ticket.Template)
	}
	if decoded, err := hex.DecodeString(ticket.Template.SHA256); err != nil || len(decoded) != 32 {
		t.Fatalf("模板来源摘要无效：%+v", ticket.Template)
	}
	loaded.Order.RequiredFiles[0] = "changed.bin"
	loaded.Order.Files[0].SHA256 = strings.Repeat("0", 64)
	*loaded.Order.Files[0].Size = 42
	*loaded.Order.Files[0].AllowEmpty = true
	loaded.Order.AllowedDirectories[0] = "changed"
	loaded.Order.ForbiddenPatterns[0] = "*.tmp"
	*loaded.Order.AllowExtraFiles = true
	loaded.Order.Components[1].Dependencies[0].AllowedVersions[0] = "9.0"
	loaded.Order.Migrations[0].Files[0] = "changed.sql"
	if ticket.RequiredFiles[0] != "app/app.bin" || ticket.Files[0].SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || *ticket.Files[0].Size != 3 || *ticket.Files[0].AllowEmpty || *ticket.AllowExtraFiles {
		t.Fatalf("修改模板影响了工单规则：%+v", ticket)
	}
	if ticket.AllowedDirectories[0] != "app" || ticket.ForbiddenPatterns[0] != "**/*.bak" || ticket.Components[1].Dependencies[0].AllowedVersions[0] != "1.0" || ticket.Migrations[0].Files[0] != "migrations/upgrade.sql" {
		t.Fatalf("修改模板影响了工单的关联规则：%+v", ticket)
	}
	orderPath := filepath.Join(filepath.Dir(path), "Delivery.json")
	if err := saveOrder(orderPath, ticket); err != nil {
		t.Fatal(err)
	}
	reloaded, _, err := loadOrder(orderPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ticket, reloaded) {
		t.Fatalf("保存后的工单规则不符：%+v", reloaded)
	}
}

func TestTemplateDigestIncludesDefaultsAndRules(t *testing.T) {
	value := templateFixture(t)
	original, err := instantiateTemplate(value, "ONE", "", "")
	if err != nil {
		t.Fatal(err)
	}
	value.Command.Args = append(value.Command.Args, "dependencies")
	changed, err := instantiateTemplate(value, "TWO", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if original.Template.SHA256 == changed.Template.SHA256 {
		t.Fatal("模板默认命令变化后必须生成不同摘要")
	}
	value.Order.Files[0].SHA256 = strings.Repeat("0", 64)
	rulesChanged, err := instantiateTemplate(value, "THREE", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if rulesChanged.Template.SHA256 == changed.Template.SHA256 {
		t.Fatal("模板内容规则变化后必须生成不同摘要")
	}
}

func TestTemplatePreservesEmptyRuleArrays(t *testing.T) {
	value := templateFixture(t)
	value.Order.RequiredFiles = []string{}
	value.Order.Files = nil
	value.Order.AllowedDirectories = []string{}
	value.Order.Components = []componentRequirement{}
	value.Order.Migrations = []migrationStep{}
	value.Order.FromVersion = "2.0"
	path := filepath.Join(templateTestDirectory(t), "empty-rules.json")
	if err := saveTemplate(path, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadTemplate(path)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := instantiateTemplate(loaded, "EMPTY-RULES", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.RequiredFiles == nil || ticket.AllowedDirectories == nil || ticket.Components == nil || ticket.Migrations == nil {
		t.Fatalf("空规则数组必须保留：%+v", ticket)
	}
}

func TestTemplateVersionOverridesValidateMigrations(t *testing.T) {
	value := templateFixture(t)
	if _, err := instantiateTemplate(value, "NEW", "2.0", "3.0"); err == nil {
		t.Fatal("版本与迁移规则冲突时必须拒绝")
	}
	value.Order.Migrations = nil
	ticket, err := instantiateTemplate(value, "NEW", "2.0", "3.0")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.FromVersion != "2.0" || ticket.ToVersion != "3.0" {
		t.Fatalf("指定版本未保存：%+v", ticket)
	}
	if _, err := instantiateTemplate(value, " ", "", ""); err == nil {
		t.Fatal("空工单号必须拒绝")
	}
}

func TestOrderJSONStrictValidation(t *testing.T) {
	for name, data := range map[string]string{
		"null":           "null",
		"array":          "[]",
		"missing_rules":  `{"order_id":"A","from_version":"1","to_version":"2","platform":"windows"}`,
		"unknown_nested": `{"order_id":"A","from_version":"1","to_version":"2","platform":"windows","required_files":[],"template":{"name":"A","version":"1","sha256":"` + strings.Repeat("0", 64) + `","unknown":true}}`,
		"trailing":       `{"order_id":"A","from_version":"1","to_version":"2","platform":"windows","required_files":[]} {}`,
		"invalid_path":   `{"order_id":"A","from_version":"1","to_version":"2","platform":"windows","required_files":["../escape"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseOrderJSON([]byte(data)); err == nil {
				t.Fatal("无效工单必须拒绝")
			}
		})
	}
	value := templateFixture(t)
	data, err := json.Marshal(value.Order)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseOrderJSON(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateRejectsInvalidDefaults(t *testing.T) {
	cases := map[string]func(*orderTemplate){
		"name":         func(value *orderTemplate) { value.Name = " " },
		"version":      func(value *orderTemplate) { value.Version = " " },
		"order":        func(value *orderTemplate) { value.Order.OrderID = "" },
		"output":       func(value *orderTemplate) { value.Output = "../upgrade.zip" },
		"empty_output": func(value *orderTemplate) { value.Output = "" },
		"input_output": func(value *orderTemplate) { value.Output = "app/app.bin" },
		"migration_output": func(value *orderTemplate) {
			value.Order.Migrations[0].Files = append(value.Order.Migrations[0].Files, "migrations/extra.sql")
			value.Output = "migrations/extra.sql"
		},
		"directory_output": func(value *orderTemplate) { value.Output = "app" },
		"tool":             func(value *orderTemplate) { value.Command.Tool = " " },
		"tool_missing":     func(value *orderTemplate) { value.Command.Tool = "" },
		"executable":       func(value *orderTemplate) { value.Command.Executable = "bin/tool.exe" },
		"workspace":        func(value *orderTemplate) { value.Command.WorkspaceTool = "../tool.exe" },
		"argument":         func(value *orderTemplate) { value.Command.Args = []string{"bad\x00argument"} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			value := templateFixture(t)
			change(&value)
			path := filepath.Join(templateTestDirectory(t), "invalid.json")
			if err := saveTemplate(path, value); err == nil {
				t.Fatal("无效模板必须拒绝")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("无效模板不能生成文件：%v", err)
			}
		})
	}
	value := templateFixture(t)
	value.Command = commandSpec{}
	if err := saveTemplate(filepath.Join(templateTestDirectory(t), "without-command.json"), value); err != nil {
		t.Fatalf("允许省略默认打包命令：%v", err)
	}
}

func TestTemplateJSONRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	value := templateFixture(t)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string][]byte{
		"null":            []byte("null"),
		"unknown":         append([]byte(`{"unknown":true,`), data[1:]...),
		"trailing":        append(append([]byte{}, data...), []byte(" {}")...),
		"unknown_command": bytes.Replace(data, []byte(`"tool":"tar.exe"`), []byte(`"tool":"tar.exe","unknown":true`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(templateTestDirectory(t), "invalid.json")
			if err := os.WriteFile(path, invalid, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadTemplate(path); err == nil {
				t.Fatal("无效模板文件必须拒绝")
			}
		})
	}
}

func TestTemplateAndOrderSaveProtectExistingFiles(t *testing.T) {
	value := templateFixture(t)
	for name, save := range map[string]func(string) error{
		"template": func(path string) error { return saveTemplate(path, value) },
		"order":    func(path string) error { return saveOrder(path, value.Order) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(templateTestDirectory(t), "existing.json")
			before := []byte("已有文件内容\n")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			if err := save(path); !errors.Is(err, os.ErrExist) {
				t.Fatalf("已有路径必须报告文件已存在：%v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("已有文件发生变化")
			}
		})
	}
}

func TestSaveOrderValidatesBeforeCreatingFile(t *testing.T) {
	value := templateFixture(t).Order
	value.RequiredFiles = append(value.RequiredFiles, "../outside")
	path := filepath.Join(templateTestDirectory(t), "invalid.json")
	if err := saveOrder(path, value); err == nil {
		t.Fatal("无效工单必须拒绝")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("无效工单不能生成文件：%v", err)
	}
}

func TestExampleTemplatePackagesRealInput(t *testing.T) {
	value, err := loadTemplate(filepath.Join("examples", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := instantiateTemplate(value, "TEMPLATE-PACKAGING", "", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := templateTestDirectory(t)
	orderPath := filepath.Join(dir, "order.json")
	if err := saveOrder(orderPath, ticket); err != nil {
		t.Fatal(err)
	}
	result, err := runPackaging(orderPath, filepath.Join("examples", "staging"), value.Output, filepath.Join(dir, "results"), value.Command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.passed() {
		t.Fatalf("模板默认命令生成的交付包未通过：命令 %+v，验收 %+v", result.Execution, result.Verification)
	}
	savedOrder, _, err := loadOrder(filepath.Join(result.Directory, orderFilename))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ticket, savedOrder) {
		t.Fatal("打包记录中的模板工单与创建的工单不符")
	}
}
