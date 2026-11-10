package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	releasePath      = "release.json"
	maxReleaseBytes  = 1 << 20
	statusPassed     = "passed"
	statusRejected   = "rejected"
	issueMissingFile = "missing_file"
	issueFileRead    = "file_unreadable"
)

type releaseMetadata struct {
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Platform    string `json:"platform"`
}

type order struct {
	OrderID string `json:"order_id"`
	releaseMetadata
	RequiredFiles      []string               `json:"required_files"`
	Files              []fileRule             `json:"files,omitempty"`
	AllowedDirectories []string               `json:"allowed_directories,omitzero"`
	ForbiddenPatterns  []string               `json:"forbidden_patterns,omitempty"`
	AllowExtraFiles    *bool                  `json:"allow_extra_files,omitempty"`
	Components         []componentRequirement `json:"components,omitzero"`
	Migrations         []migrationStep        `json:"migrations,omitzero"`
	Template           *templateReference     `json:"template,omitempty"`
}

type issue struct {
	Code         string `json:"code"`
	Path         string `json:"path,omitempty"`
	Field        string `json:"field,omitempty"`
	Expected     string `json:"expected,omitempty"`
	Actual       string `json:"actual,omitempty"`
	Message      string `json:"message"`
	InputPresent *bool  `json:"input_present,omitempty"`
	Advice       string `json:"advice,omitempty"`
}

type verification struct {
	OrderID         string              `json:"order_id"`
	ZIPPath         string              `json:"zip_path"`
	Status          string              `json:"status"`
	ArchiveReadable bool                `json:"archive_readable"`
	Issues          []issue             `json:"issues"`
	Files           []verifiedFile      `json:"files,omitempty"`
	Checks          []verificationCheck `json:"checks,omitempty"`
}

func loadOrder(path string) (order, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return order{}, nil, fmt.Errorf("读取工单 %q 失败 %w", path, err)
	}
	ticket, err := parseOrderJSON(data)
	if err != nil {
		return order{}, nil, fmt.Errorf("解析工单 %q 失败 %w", path, err)
	}
	return ticket, data, nil
}

func validRelativePath(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:<>\"|?*\x00") {
		return false
	}
	if _, err := filepath.Localize(name); err != nil {
		return false
	}
	for _, character := range name {
		if character < 32 {
			return false
		}
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasSuffix(segment, " ") || strings.HasSuffix(segment, ".") {
			return false
		}
		base := strings.TrimRight(strings.ToUpper(strings.SplitN(segment, ".", 2)[0]), " ")
		if _, err := filepath.Localize(base); err != nil {
			return false
		}
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return false
		}
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func newVerification(ticket order, zipPath string) verification {
	return verification{OrderID: ticket.OrderID, ZIPPath: zipPath, Status: statusPassed, Issues: []issue{}}
}

func (report *verification) reject(problem issue) {
	report.Status = statusRejected
	report.Issues = append(report.Issues, problem)
}

func verifyPackage(orderPath, zipPath string) (verification, error) {
	ticket, _, err := loadOrder(orderPath)
	if err != nil {
		return verification{}, err
	}
	actualPath, err := filepath.Abs(zipPath)
	if err != nil {
		return verification{}, err
	}
	return verifyZIP(ticket, actualPath), nil
}

func verifyZIP(ticket order, zipPath string) verification {
	report := newVerification(ticket, zipPath)
	if err := validateOrder(ticket); err != nil {
		report.reject(issue{Code: "invalid_order", Message: err.Error()})
		return report
	}
	archive, err := openZIP(zipPath)
	if err != nil {
		report.reject(issue{Code: "zip_unreadable", Message: fmt.Sprintf("读取 ZIP 失败 %v", err)})
		report.completeChecks(ticket)
		return report
	}
	defer archive.Close()

	report.ArchiveReadable = true
	entries := make(map[string][]*zip.File)
	for _, entry := range archive.File {
		entries[entry.Name] = append(entries[entry.Name], entry)
	}
	checkArchivePolicy(&report, ticket, archive.File)
	for _, name := range requiredPaths(ticket) {
		if len(entries[name]) == 0 {
			report.reject(issue{Code: issueMissingFile, Path: name, Message: fmt.Sprintf("缺少文件 %q", name)})
		}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		checkEntry(&report, ticket, name, entries[name])
	}
	report.completeChecks(ticket)
	return report
}

func openZIP(path string) (*zip.ReadCloser, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q 必须为普通文件", path)
	}
	archive, err := zip.OpenReader(path)
	if err != nil && archive != nil {
		archive.Close()
		return nil, err
	}
	return archive, err
}

func checkEntry(report *verification, ticket order, name string, entries []*zip.File) {
	if len(entries) == 0 {
		report.reject(issue{Code: issueMissingFile, Path: name, Message: fmt.Sprintf("缺少文件 %q", name)})
		return
	}
	if len(entries) > 1 {
		report.reject(issue{Code: issueFileRead, Path: name, Message: fmt.Sprintf("ZIP 内重复出现路径 %q", name)})
	}
	for _, entry := range entries {
		readEntry(report, ticket, name, entry, len(entries) == 1)
	}
}

func readEntry(report *verification, ticket order, name string, entry *zip.File, unique bool) {
	if entry.FileInfo().IsDir() {
		if entry.Mode().Type() != fs.ModeDir || !strings.HasSuffix(name, "/") || entry.UncompressedSize64 != 0 {
			report.reject(issue{Code: issueFileRead, Path: name, Message: fmt.Sprintf("ZIP 目录 %q 必须使用目录名称和空内容", name)})
			return
		}
		content, err := entry.Open()
		if err != nil {
			report.reject(fileReadIssue(name, err))
			return
		}
		_, readErr := io.Copy(io.Discard, content)
		if err := errors.Join(readErr, content.Close()); err != nil {
			report.reject(fileReadIssue(name, err))
		}
		return
	}
	actual := verifiedFile{Path: name}
	defer func() { report.Files = append(report.Files, actual) }()
	if !entry.Mode().IsRegular() {
		report.reject(issue{Code: issueFileRead, Path: name, Message: fmt.Sprintf("ZIP 中的 %q 必须为普通文件", name)})
		return
	}
	content, err := entry.Open()
	if err != nil {
		report.reject(fileReadIssue(name, err))
		return
	}
	digest := sha256.New()
	var metadata bytes.Buffer
	var destination io.Writer = digest
	if name == releasePath && entry.UncompressedSize64 <= maxReleaseBytes {
		destination = io.MultiWriter(digest, &metadata)
	}
	size, readErr := io.Copy(destination, content)
	if err := errors.Join(readErr, content.Close()); err != nil {
		report.reject(fileReadIssue(name, err))
		return
	}
	actual.Readable = true
	actual.Size = size
	actual.SHA256 = hex.EncodeToString(digest.Sum(nil))
	checkFileRules(report, ticket, actual)
	if name == releasePath && unique {
		if size > maxReleaseBytes {
			report.reject(issue{Code: "invalid_release", Path: name, Message: "release.json 超过 1 MiB"})
			return
		}
		checkRelease(report, ticket, metadata.Bytes())
	}
}

func fileReadIssue(name string, err error) issue {
	return issue{Code: issueFileRead, Path: name, Message: fmt.Sprintf("读取文件 %q 失败 %v", name, err)}
}

func checkRelease(report *verification, ticket order, data []byte) {
	if len(data) > maxReleaseBytes {
		report.reject(issue{Code: "invalid_release", Path: releasePath, Message: "release.json 超过 1 MiB"})
		return
	}
	var metadata *releaseDeclaration
	if err := json.Unmarshal(data, &metadata); err != nil {
		report.reject(issue{Code: "invalid_release", Path: releasePath, Message: fmt.Sprintf("解析 release.json 失败 %v", err)})
		return
	}
	if metadata == nil {
		report.reject(issue{Code: "invalid_release", Path: releasePath, Message: "release.json 必须为 JSON 对象"})
		return
	}

	for _, comparison := range []struct {
		field    string
		expected string
		actual   string
	}{
		{"from_version", ticket.FromVersion, metadata.FromVersion},
		{"to_version", ticket.ToVersion, metadata.ToVersion},
		{"platform", ticket.Platform, metadata.Platform},
	} {
		if comparison.expected != comparison.actual {
			report.reject(issue{
				Code:     "metadata_mismatch",
				Path:     releasePath,
				Field:    comparison.field,
				Expected: comparison.expected,
				Actual:   comparison.actual,
				Message:  fmt.Sprintf("release.json 的 %s 不匹配，工单要求 %q，包内声明 %q", comparison.field, comparison.expected, comparison.actual),
			})
		}
	}
	checkComponents(report, ticket, metadata.Components)
	checkMigrations(report, ticket, metadata.Migrations)
}

func (report *verification) diagnoseMissing(inputFiles map[string]struct{}) {
	for index, problem := range report.Issues {
		if problem.Code != issueMissingFile {
			continue
		}
		_, present := inputFiles[problem.Path]
		report.Issues[index].InputPresent = &present
		if present {
			report.Issues[index].Advice = "打包前存在，成品里缺。建议检查打包步骤。"
		} else {
			report.Issues[index].Advice = "打包前已缺少该文件。建议检查输入准备。"
		}
	}
}

func (report verification) writeTo(out io.Writer) {
	fmt.Fprintf(out, "工单 %s\nZIP %s\n", report.OrderID, report.ZIPPath)
	if report.Status == statusPassed {
		fmt.Fprintln(out, "验收通过")
		return
	}
	fmt.Fprintln(out, "验收拒收")
	for _, problem := range report.Issues {
		fmt.Fprintln(out, "-", problem.Message)
		if problem.Advice != "" {
			fmt.Fprintln(out, " ", problem.Advice)
		}
	}
}

func writeMissingComparison(out io.Writer, original, current verification) {
	currentIssues := make(map[string]string)
	for _, problem := range current.Issues {
		if problem.Code == issueMissingFile || problem.Code == issueFileRead {
			currentIssues[problem.Path] = problem.Code
		}
	}
	hasMissing := false
	for _, problem := range original.Issues {
		if problem.Code != issueMissingFile {
			continue
		}
		hasMissing = true
		switch {
		case !current.ArchiveReadable:
			fmt.Fprintf(out, "原缺项 %q 无法比较，本次 ZIP 无法读取\n", problem.Path)
		case currentIssues[problem.Path] == issueMissingFile:
			fmt.Fprintf(out, "原缺项 %q 再次缺少\n", problem.Path)
		case currentIssues[problem.Path] == issueFileRead:
			fmt.Fprintf(out, "原缺项 %q 本次存在，文件无法读取\n", problem.Path)
		default:
			fmt.Fprintf(out, "原缺项 %q 本次已包含\n", problem.Path)
		}
	}
	if !hasMissing {
		fmt.Fprintln(out, "原验收结果没有缺少的文件")
	}
}
