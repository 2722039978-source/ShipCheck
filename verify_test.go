package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rulesZIPEntry struct {
	name string
	data string
}

func rulesDirectory(t *testing.T) string {
	t.Helper()
	base, err := filepath.Abs(".local/rules-tests")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	return directory
}

func rulesZIP(t *testing.T, entries []rulesZIPEntry) string {
	t.Helper()
	name := filepath.Join(rulesDirectory(t), "package.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		output, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func rulesOrder(t *testing.T, extra map[string]any) order {
	t.Helper()
	data := rulesOrderJSON(t, extra)
	name := filepath.Join(rulesDirectory(t), "order.json")
	if err := os.WriteFile(name, data, 0644); err != nil {
		t.Fatal(err)
	}
	ticket, _, err := loadOrder(name)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func rulesOrderJSON(t *testing.T, extra map[string]any) []byte {
	t.Helper()
	value := map[string]any{
		"order_id": "RULE-001", "from_version": "1.0", "to_version": "2.0",
		"platform": "windows-amd64", "required_files": []string{"payload/app.bin"},
	}
	for key, item := range extra {
		value[key] = item
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

const rulesRelease = `{"from_version":"1.0","to_version":"2.0","platform":"windows-amd64"}`

func rulesDigest(data string) string {
	digest := sha256.Sum256([]byte(data))
	return hex.EncodeToString(digest[:])
}

func rulesAssertIssue(t *testing.T, report verification, code, path string) {
	t.Helper()
	for _, problem := range report.Issues {
		if problem.Code == code && (path == "" || problem.Path == path) {
			if report.Status != statusRejected {
				t.Fatalf("问题存在但状态为 %q", report.Status)
			}
			return
		}
	}
	t.Fatalf("缺少问题 code=%s path=%s，实际为 %+v", code, path, report)
}

func TestRulesContentValidation(t *testing.T) {
	cases := []struct {
		name    string
		content string
		rule    map[string]any
		code    string
	}{
		{"correct", "payload", map[string]any{"sha256": rulesDigest("payload"), "size": 7, "allow_empty": false}, ""},
		{"hash", "different", map[string]any{"sha256": rulesDigest("payload")}, "sha256_mismatch"},
		{"size", "payload", map[string]any{"size": 6}, "size_mismatch"},
		{"empty", "", map[string]any{"allow_empty": false}, "empty_file"},
		{"allowed-empty", "", map[string]any{"allow_empty": true, "size": 0, "sha256": rulesDigest("")}, ""},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			item.rule["path"] = "payload/app.bin"
			ticket := rulesOrder(t, map[string]any{"files": []any{item.rule}})
			archive := rulesZIP(t, []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", item.content}})
			report := verifyZIP(ticket, archive)
			if item.code != "" {
				rulesAssertIssue(t, report, item.code, "payload/app.bin")
			} else if report.Status != statusPassed {
				t.Fatalf("有效内容验收失败 %+v", report)
			}
		})
	}
}

func TestRulesArchivePolicy(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
		entry rulesZIPEntry
		code  string
	}{
		{"unexpected", map[string]any{"allow_extra_files": false}, rulesZIPEntry{"extra.txt", "extra"}, "extra_file"},
		{"directory", map[string]any{"allowed_directories": []string{"payload"}}, rulesZIPEntry{"other/config.json", "{}"}, "directory_not_allowed"},
		{"forbidden", map[string]any{"forbidden_patterns": []string{"payload/*.bak"}}, rulesZIPEntry{"payload/source.bak", "backup"}, "forbidden_path"},
		{"traversal", nil, rulesZIPEntry{"../outside.txt", "unsafe"}, "invalid_path"},
		{"backslash", nil, rulesZIPEntry{`payload\outside.txt`, "unsafe"}, "invalid_path"},
		{"duplicate-extra", nil, rulesZIPEntry{"extra.txt", "duplicate"}, "duplicate_path"},
		{"case-collision", nil, rulesZIPEntry{"PAYLOAD/APP.BIN", "duplicate"}, "duplicate_path"},
		{"file-directory-collision", nil, rulesZIPEntry{"payload/app.bin/child.txt", "nested"}, "path_conflict"},
		{"reserved-name", nil, rulesZIPEntry{"payload/CON.txt", "unsafe"}, "invalid_path"},
		{"reserved-device", nil, rulesZIPEntry{"payload/COM¹.txt", "unsafe"}, "invalid_path"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			ticket := rulesOrder(t, item.extra)
			entries := []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", "payload"}}
			if item.name == "duplicate-extra" {
				entries = append(entries, rulesZIPEntry{"extra.txt", "original"})
			}
			entries = append(entries, item.entry)
			report := verifyZIP(ticket, rulesZIP(t, entries))
			rulesAssertIssue(t, report, item.code, item.entry.name)
		})
	}
}

func TestRulesListedContentAndLegacy(t *testing.T) {
	ticket := rulesOrder(t, map[string]any{"files": []any{map[string]any{"path": "payload/config.json", "size": 2}}})
	report := verifyZIP(ticket, rulesZIP(t, []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", ""}}))
	rulesAssertIssue(t, report, issueMissingFile, "payload/config.json")
	legacy := rulesOrder(t, nil)
	report = verifyZIP(legacy, rulesZIP(t, []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", ""}, {"extra.txt", "ok"}}))
	if report.Status != statusPassed {
		t.Fatalf("原工单兼容性检查失败 %+v", report)
	}
}

func TestRulesRejectMalformedOrder(t *testing.T) {
	cases := []map[string]any{
		{"files": []any{map[string]any{"path": "payload/app.bin", "sha256": "invalid"}}},
		{"files": []any{map[string]any{"path": "payload/app.bin", "size": -1}}},
		{"files": []any{map[string]any{"path": "payload/app.bin"}, map[string]any{"path": "PAYLOAD/APP.BIN"}}},
		{"forbidden_patterns": []string{"["}},
		{"allowed_directories": []string{"../outside"}},
		{"required_files": []string{"payload/app.bin", "PAYLOAD/APP.BIN"}},
		{"components": []any{map[string]any{"name": "app", "version": ""}}},
		{"components": []any{map[string]any{"name": "app", "version": "2.0", "dependencies": []any{map[string]any{"name": "runtime", "allowed_versions": []string{}}}}}},
		{"migrations": []any{map[string]any{"id": "step", "from_version": "1.5", "to_version": "2.0", "files": []string{"migration.sql"}}}},
	}
	for index, extra := range cases {
		name := filepath.Join(rulesDirectory(t), "order.json")
		if err := os.WriteFile(name, rulesOrderJSON(t, extra), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrder(name); err == nil {
			t.Errorf("无效工单 %d 被接受：%v", index, extra)
		}
	}
}

func TestRulesReadsAllZIPFiles(t *testing.T) {
	ticket := rulesOrder(t, nil)
	name := filepath.Join(rulesDirectory(t), "corrupt.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", "payload"}} {
		content, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := content.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	content, err := writer.CreateRaw(&zip.FileHeader{Name: "unchecked.txt", Method: zip.Store, CRC32: 123, CompressedSize64: 6, UncompressedSize64: 6})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := content.Write([]byte("broken")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	rulesAssertIssue(t, verifyZIP(ticket, name), issueFileRead, "unchecked.txt")
}

func TestRulesReleaseLimits(t *testing.T) {
	ticket := rulesOrder(t, nil)
	for _, content := range []string{"null", strings.Repeat(" ", maxReleaseBytes+1)} {
		report := verifyZIP(ticket, rulesZIP(t, []rulesZIPEntry{{releasePath, content}, {"payload/app.bin", "payload"}}))
		rulesAssertIssue(t, report, "invalid_release", releasePath)
	}
}

func TestRulesSpecialFileTypes(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeDir | 0755, os.ModeSymlink | 0777} {
		name := filepath.Join(rulesDirectory(t), "special.zip")
		file, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(file)
		metadata, err := writer.Create(releasePath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := metadata.Write([]byte(rulesRelease)); err != nil {
			t.Fatal(err)
		}
		header := &zip.FileHeader{Name: "payload/app.bin", Method: zip.Store}
		header.SetMode(mode)
		if _, err := writer.CreateHeader(header); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		rulesAssertIssue(t, verifyZIP(rulesOrder(t, nil), name), issueFileRead, "payload/app.bin")
	}
}

func TestRulesFileInventory(t *testing.T) {
	ticket := rulesOrder(t, nil)
	report := verifyZIP(ticket, rulesZIP(t, []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", "payload"}, {"extra.txt", "extra"}}))
	if len(report.Files) != 3 {
		t.Fatalf("文件清单数量不匹配 %+v", report.Files)
	}
	for _, file := range report.Files {
		if !file.Readable || len(file.SHA256) != 64 {
			t.Fatalf("文件读取证据缺失 %+v", file)
		}
		if file.Path == "payload/app.bin" && (file.Size != 7 || file.SHA256 != rulesDigest("payload")) {
			t.Fatalf("文件摘要或字节数不匹配 %+v", file)
		}
	}
	if len(report.Checks) != 5 {
		t.Fatalf("检查范围缺失 %+v", report.Checks)
	}
}

func TestRulesPolicySerialization(t *testing.T) {
	ticket := rulesOrder(t, map[string]any{"required_files": []string{}, "allowed_directories": []string{}, "components": []any{}})
	data, err := json.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := parseOrderJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	report := verifyZIP(copied, rulesZIP(t, []rulesZIPEntry{{releasePath, rulesRelease}, {"payload/app.bin", "payload"}}))
	rulesAssertIssue(t, report, "directory_not_allowed", "payload/app.bin")
	if copied.Components == nil {
		t.Fatal("components 空数组策略未保存")
	}
}

func TestRulesStrictExample(t *testing.T) {
	ticket, _, err := loadOrder("examples/strict-order.json")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile("examples/strict-release.json")
	if err != nil {
		t.Fatal(err)
	}
	entries := []rulesZIPEntry{{releasePath, string(metadata)}, {"payload/message.txt", "hello"}, {"migrations/001.sql", "SELECT 1;"}, {"migrations/002.sql", "SELECT 2;"}}
	report := verifyZIP(ticket, rulesZIP(t, entries))
	if report.Status != statusPassed {
		t.Fatalf("示例验收失败 %+v", report)
	}
}
