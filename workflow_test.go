package main

import (
	"os"
	"path/filepath"
	"testing"
)

func packagingFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	staging := filepath.Join(dir, "staging")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	orderPath := filepath.Join(dir, "order.json")
	if err := os.WriteFile(orderPath, []byte(`{"order_id":"TEST-001","from_version":"1","to_version":"2","platform":"windows","required_files":["app.bin"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "release.json"), []byte(`{"from_version":"1","to_version":"2","platform":"windows"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "app.bin"), []byte("application"), 0600); err != nil {
		t.Fatal(err)
	}
	return orderPath, staging, filepath.Join(dir, "results")
}

func packagingCommand() commandSpec {
	return commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "release.json", "app.bin"}}
}

func TestRunPreservesReportAndInput(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	result, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil {
		t.Fatal(err)
	}
	if !result.passed() {
		t.Fatalf("打包应通过：%+v", result)
	}
	for _, name := range []string{"report.html", "receipt.json", "input/app.bin"} {
		if _, err := os.Stat(filepath.Join(result.Directory, name)); err != nil {
			t.Errorf("缺少保存内容 %s：%v", name, err)
		}
	}
}

func TestRunWithoutZIPRecordsChecks(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	writeUnavailableCheckOrder(t, orderPath)
	command := commandSpec{Tool: "cmd.exe", Args: []string{"/c", "exit /b 0"}}
	result, err := runPackaging(orderPath, staging, "delivery.zip", results, command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Execution.passed() || result.passed() || result.Verification.ArchiveReadable {
		t.Fatalf("命令未生成ZIP时应拒收：%+v", result)
	}
	assertUnavailableChecks(t, result.Directory, result.Verification)
}
