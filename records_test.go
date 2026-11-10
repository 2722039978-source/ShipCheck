package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifySavesZIPHashAndEscapedReport(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	if err := os.WriteFile(orderPath, []byte(`{"order_id":"<script>alert(1)</script>","from_version":"1","to_version":"2","platform":"windows","required_files":["app.bin"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	run, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := verifyAndRecord(orderPath, run.Verification.ZIPPath, results)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(run.Verification.ZIPPath)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(saved.Report.ZIPPath)
	if err != nil || !bytes.Equal(data, copied) {
		t.Fatalf("验收ZIP副本不符：%v", err)
	}
	receipt, err := readJSON[receiptRecord](filepath.Join(saved.Directory, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if receipt.ZIPSHA256 != hex.EncodeToString(digest[:]) || receipt.Kind != "verify" || receipt.Order.OrderID != "<script>alert(1)</script>" || receipt.CreatedAt == "" || receipt.ToolVersion == "" {
		t.Fatalf("验收记录不完整：%+v", receipt)
	}
	html, err := os.ReadFile(filepath.Join(saved.Directory, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>alert(1)</script>") || !strings.Contains(string(html), "&lt;script&gt;") || !strings.Contains(string(html), receipt.ZIPSHA256) {
		t.Fatal("报告缺少摘要或者HTML转义")
	}
}

func TestHistoryFiltersAndLegacyTime(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil {
		t.Fatal(err)
	}
	failed := commandSpec{Tool: "cmd.exe", Args: []string{"/c", "tar.exe -a -cf delivery.zip release.json app.bin & exit /b 7"}}
	second, err := runPackaging(orderPath, staging, "delivery.zip", results, failed)
	if err != nil {
		t.Fatal(err)
	}
	if second.Verification.Status != statusPassed || second.passed() {
		t.Fatal("命令失败的总体状态必须为拒收，ZIP应通过")
	}
	entries, err := listHistory(results, historyFilter{OrderID: "TEST-001", Version: "2", Status: statusRejected, FromDate: time.Now().Format("2006-01-02"), ToDate: time.Now().Format("2006-01-02")})
	if err != nil || len(entries) != 1 || entries[0].Directory != second.Directory {
		t.Fatalf("历史筛选不符：%+v，%v", entries, err)
	}
	if err := os.Remove(filepath.Join(first.Directory, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	entries, err = listHistory(results, historyFilter{Status: statusPassed})
	if err != nil || len(entries) != 1 || entries[0].Kind != "legacy" || entries[0].CreatedAt != "" || entries[0].TimeNotice == "" {
		t.Fatalf("旧记录时间应明确显示资料不足：%+v，%v", entries, err)
	}
	entries, err = listHistory(results, historyFilter{Status: statusPassed, FromDate: "2000-01-01"})
	if err != nil || len(entries) != 0 {
		t.Fatalf("日期筛选应排除没有可靠时间的旧记录：%+v，%v", entries, err)
	}
	if _, err := listHistory(results, historyFilter{FromDate: "invalid"}); err == nil {
		t.Fatal("无效日期必须报错")
	}
}

func TestHistoryUsesLocalCalendarDate(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	result, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil {
		t.Fatal(err)
	}
	receipt := result.Receipt
	receipt.CreatedAt = time.Date(2000, 1, 2, 0, 30, 0, 0, time.Local).UTC().Format(time.RFC3339Nano)
	if err := writeJSON(filepath.Join(result.Directory, "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	entries, err := listHistory(results, historyFilter{FromDate: "2000-01-02", ToDate: "2000-01-02"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("当地日期应包含凌晨记录：%+v，%v", entries, err)
	}
}

func TestVerifyUnavailableZIPRecordsChecks(t *testing.T) {
	for _, kind := range []string{"missing", "directory"} {
		t.Run(kind, func(t *testing.T) {
			orderPath, staging, results := packagingFixture(t)
			writeUnavailableCheckOrder(t, orderPath)
			zipPath := filepath.Join(staging, "missing.zip")
			if kind == "directory" {
				zipPath = staging
			}
			saved, err := verifyAndRecord(orderPath, zipPath, results)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Report.Status != statusRejected || saved.Report.ArchiveReadable {
				t.Fatalf("无效ZIP路径应拒收：%+v", saved.Report)
			}
			assertUnavailableChecks(t, saved.Directory, saved.Report)
		})
	}
}

func writeUnavailableCheckOrder(t *testing.T, path string) {
	t.Helper()
	data := []byte(`{"order_id":"TEST-001","from_version":"1","to_version":"2","platform":"windows","required_files":["app.bin"],"components":[{"name":"app","version":"2"}],"migrations":[{"id":"update","from_version":"1","to_version":"2","files":["app.bin"]}]}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertUnavailableChecks(t *testing.T, directory string, report verification) {
	t.Helper()
	savedReport, err := readJSON[verification](filepath.Join(directory, verificationFilename))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := readJSON[receiptRecord](filepath.Join(directory, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []verification{report, savedReport, receipt.Verification} {
		if len(current.Checks) != 5 {
			t.Fatalf("验收记录应包含全部检查项：%+v", current.Checks)
		}
		checks := make(map[string]string)
		for _, check := range current.Checks {
			checks[check.Name] = check.Status
		}
		for name, expected := range map[string]string{"archive": statusRejected, "files": "not_run", "metadata": "not_run", "components": "not_run", "migrations": "not_run"} {
			if checks[name] != expected {
				t.Errorf("检查 %s 状态为 %q，要求 %q", name, checks[name], expected)
			}
		}
	}
	html, err := os.ReadFile(filepath.Join(directory, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"files", "metadata", "components", "migrations"} {
		if !strings.Contains(string(html), "<td>"+name+"</td><td>not_run</td>") {
			t.Errorf("HTML 缺少尚未执行的检查：%s", name)
		}
	}
}
