package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReviseRetainsIndependentEvidenceAndContentChanges(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	wrong := commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "release.json"}}
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, wrong)
	if err != nil || first.passed() {
		t.Fatalf("缺少文件的包应拒收：%v", err)
	}
	originalOrder, err := os.ReadFile(filepath.Join(first.Directory, orderFilename))
	if err != nil {
		t.Fatal(err)
	}
	originalZIP, err := os.ReadFile(first.Verification.ZIPPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "app.bin"), []byte("revised application"), 0600); err != nil {
		t.Fatal(err)
	}
	second, _, err := revisePackaging(first.Directory, staging, results, nil)
	if err != nil || second.passed() {
		t.Fatalf("持续缺少文件应拒收：%v", err)
	}
	if len(second.Receipt.Comparison.Persisting) != 1 || len(second.Receipt.Comparison.InputChanges) != 1 {
		t.Fatalf("输入和持续问题比较不符：%+v", second.Receipt.Comparison)
	}
	command := packagingCommand()
	third, _, err := revisePackaging(second.Directory, "", results, &command)
	if err != nil || !third.passed() {
		t.Fatalf("连续修正应通过：%+v，%v", third, err)
	}
	if len(third.Receipt.Comparison.Resolved) != 1 || len(third.Receipt.Comparison.ZIPChanges) != 1 || third.Receipt.Comparison.ZIPChanges[0].Path != "app.bin" {
		t.Fatalf("修正问题和ZIP内容比较不符：%+v", third.Receipt.Comparison)
	}
	for _, path := range []string{filepath.Join(first.Directory, orderFilename), first.Verification.ZIPPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected := originalZIP
		if filepath.Base(path) == orderFilename {
			expected = originalOrder
		}
		if !bytes.Equal(data, expected) {
			t.Fatalf("原证据受到修改：%s", path)
		}
	}
	for _, result := range []attempt{first, second, third} {
		if _, err := os.Stat(filepath.Join(result.Directory, inputDirectory, "app.bin")); err != nil {
			t.Fatal(err)
		}
	}
	if third.Receipt.SourceDirectory != second.Directory {
		t.Fatal("修正来源不符")
	}
	originalInput, err := os.ReadFile(filepath.Join(first.Directory, inputDirectory, "app.bin"))
	if err != nil || string(originalInput) != "application" {
		t.Fatalf("原输入快照受到修改：%s，%v", originalInput, err)
	}
	currentInput, err := os.ReadFile(filepath.Join(third.Directory, inputDirectory, "app.bin"))
	if err != nil || string(currentInput) != "revised application" {
		t.Fatalf("连续修正没有保留新输入：%s，%v", currentInput, err)
	}
}

func TestUnreadableRevisionDoesNotResolveProblems(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	wrong := commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "release.json"}}
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, wrong)
	if err != nil {
		t.Fatal(err)
	}
	command := commandSpec{Tool: "cmd.exe", Args: []string{"/c", "echo invalid>delivery.zip"}}
	current, _, err := revisePackaging(first.Directory, "", results, &command)
	if err != nil {
		t.Fatal(err)
	}
	comparison := current.Receipt.Comparison
	if len(comparison.Resolved) != 0 || len(comparison.Unverified) != 1 || len(comparison.Added) != 1 || comparison.ZIPComparisonAvailable {
		t.Fatalf("无法读取ZIP时问题比较不符：%+v", comparison)
	}
}

func TestReplayProducesReusableSnapshot(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	wrong := commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "release.json"}}
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, wrong)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(first.Directory, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	second, _, err := replayPackaging(first.Directory, results)
	if err != nil {
		t.Fatal(err)
	}
	command := packagingCommand()
	third, _, err := revisePackaging(second.Directory, "", results, &command)
	if err != nil || !third.passed() {
		t.Fatalf("旧证据复跑后应能继续修正：%v", err)
	}
}

func TestMissingReleaseKeepsMetadataIssueUnverified(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	if err := os.WriteFile(filepath.Join(staging, "release.json"), []byte(`{"from_version":"1","to_version":"9","platform":"windows"}`), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil || first.passed() {
		t.Fatalf("错误版本应拒收：%v", err)
	}
	command := commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "app.bin"}}
	current, _, err := revisePackaging(first.Directory, "", results, &command)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Receipt.Comparison.Resolved) != 0 || len(current.Receipt.Comparison.Unverified) != 1 {
		t.Fatalf("缺少release.json时无法确认版本问题：%+v", current.Receipt.Comparison)
	}
}

func TestRevisionComparesResolvedPersistingAndAddedIssues(t *testing.T) {
	orderPath, staging, results := packagingFixture(t)
	if err := os.WriteFile(filepath.Join(staging, "release.json"), []byte(`{"from_version":"1","to_version":"9","platform":"linux"}`), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := runPackaging(orderPath, staging, "delivery.zip", results, packagingCommand())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "release.json"), []byte(`{"from_version":"1","to_version":"8","platform":"windows"}`), 0600); err != nil {
		t.Fatal(err)
	}
	command := commandSpec{Tool: "tar.exe", Args: []string{"-a", "-cf", "delivery.zip", "release.json"}}
	current, _, err := revisePackaging(first.Directory, staging, results, &command)
	if err != nil {
		t.Fatal(err)
	}
	comparison := current.Receipt.Comparison
	if len(comparison.Resolved) != 1 || comparison.Resolved[0].Field != "platform" ||
		len(comparison.Persisting) != 1 || comparison.Persisting[0].Field != "to_version" || comparison.Persisting[0].Actual != "8" ||
		len(comparison.Added) != 1 || comparison.Added[0].Path != "app.bin" || len(comparison.Unverified) != 0 {
		t.Fatalf("完整问题比较不符：%+v", comparison)
	}
}
