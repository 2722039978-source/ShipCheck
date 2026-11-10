//go:build !shipcheck_gui

package main

import (
	"path/filepath"
	"testing"
)

func TestCLIOrderValidation(t *testing.T) {
	if code := dispatch([]string{"order", "validate", "--order", "examples/order.json"}); code != exitPassed {
		t.Fatalf("有效工单应通过，退出码 %d", code)
	}
	if code := dispatch([]string{"order", "validate", "--order", "missing-order.json"}); code != exitError {
		t.Fatalf("不存在的工单应报告错误，退出码 %d", code)
	}
}

func TestCLITemplateCreatesIndependentOrder(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "template.json")
	orderPath := filepath.Join(dir, "delivery.json")
	if code := dispatch([]string{"template", "save", "--name", "示例产品", "--version", "1", "--order", "examples/order.json", "--output", templatePath}); code != exitPassed {
		t.Fatalf("保存模板失败，退出码 %d", code)
	}
	if code := dispatch([]string{"order", "create", "--template", templatePath, "--id", "NEW-001", "--from", "1.6", "--to", "1.7", "--output", orderPath}); code != exitPassed {
		t.Fatalf("创建工单失败，退出码 %d", code)
	}
	ticket, _, err := loadOrder(orderPath)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.OrderID != "NEW-001" || len(ticket.RequiredFiles) != 3 {
		t.Fatalf("新工单内容不符：%+v", ticket)
	}
	if code := dispatch([]string{"order", "create", "--template", templatePath, "--id", "OTHER", "--output", orderPath}); code != exitError {
		t.Fatalf("已有工单应受到覆盖保护，退出码 %d", code)
	}
}

func TestCLIEmptyHistory(t *testing.T) {
	if code := dispatch([]string{"history", "--results", t.TempDir(), "--json"}); code != exitPassed {
		t.Fatalf("空历史查询失败，退出码 %d", code)
	}
}
