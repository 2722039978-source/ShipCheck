package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const toolVersion = "0.2.0"

type savedVerification struct {
	Directory string
	Report    verification
}

type receiptRecord struct {
	SchemaVersion   int                `json:"schema_version"`
	ToolVersion     string             `json:"tool_version"`
	CreatedAt       string             `json:"created_at"`
	Kind            string             `json:"kind"`
	Status          string             `json:"status"`
	Order           order              `json:"order"`
	ZIPPath         string             `json:"zip_path,omitempty"`
	ZIPSHA256       string             `json:"zip_sha256,omitempty"`
	SourceDirectory string             `json:"source_directory,omitempty"`
	SourceZIPPath   string             `json:"source_zip_path,omitempty"`
	Execution       *executionRecord   `json:"execution,omitempty"`
	Verification    verification       `json:"verification"`
	Comparison      *attemptComparison `json:"comparison,omitempty"`
}

type historyFilter struct {
	OrderID  string
	Version  string
	Status   string
	FromDate string
	ToDate   string
}

type historyEntry struct {
	Directory   string `json:"directory"`
	OrderID     string `json:"order_id"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Status      string `json:"status"`
	Kind        string `json:"kind"`
	CreatedAt   string `json:"created_at"`
	ZIPPath     string `json:"zip_path"`
	TimeNotice  string `json:"time_notice,omitempty"`
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func makeReceipt(ticket order, report verification, kind string) receiptRecord {
	return receiptRecord{
		SchemaVersion: 1,
		ToolVersion:   toolVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Kind:          kind,
		Status:        report.Status,
		Order:         ticket,
		Verification:  report,
	}
}

func finishAttempt(result *attempt, ticket order, kind, source string, comparison *attemptComparison) error {
	receipt := makeReceipt(ticket, result.Verification, kind)
	receipt.SourceDirectory = source
	receipt.Execution = &result.Execution
	receipt.Comparison = comparison
	if !result.passed() {
		receipt.Status = statusRejected
	}
	if err := recordArtifact(&receipt, result.Directory); err != nil {
		return err
	}
	result.Receipt = receipt
	return saveReceipt(result.Directory, receipt)
}

func recordArtifact(receipt *receiptRecord, dir string) error {
	artifact := filepath.Join(dir, "artifact")
	relative, err := filepath.Rel(artifact, receipt.Verification.ZIPPath)
	if err != nil || !filepath.IsLocal(relative) {
		return nil
	}
	digest, err := fileSHA256(receipt.Verification.ZIPPath)
	if err != nil {
		return fmt.Errorf("计算交付包摘要失败 %w", err)
	}
	receipt.ZIPPath = receipt.Verification.ZIPPath
	receipt.ZIPSHA256 = digest
	return nil
}

func saveReceipt(dir string, receipt receiptRecord) error {
	if err := writeJSON(filepath.Join(dir, "receipt.json"), receipt); err != nil {
		return err
	}
	return writeHTMLReport(filepath.Join(dir, "report.html"), receipt)
}

func verifyAndRecord(orderPath, zipPath, resultsPath string) (saved savedVerification, err error) {
	ticket, data, err := loadOrder(orderPath)
	if err != nil {
		return saved, err
	}
	source, err := filepath.Abs(zipPath)
	if err != nil {
		return saved, err
	}
	dir, err := newResultDirectory(resultsPath, source, "verify-")
	if err != nil {
		return saved, err
	}
	saved.Directory = dir
	defer func() {
		if err != nil {
			err = fmt.Errorf("验收结果目录 %q\n%w", dir, err)
		}
	}()
	if err := os.WriteFile(filepath.Join(dir, orderFilename), data, 0600); err != nil {
		return saved, err
	}
	info, sourceErr := os.Stat(source)
	if sourceErr == nil && !info.Mode().IsRegular() {
		sourceErr = fmt.Errorf("%q 必须为普通文件", source)
	}
	if sourceErr != nil {
		saved.Report = newVerification(ticket, source)
		saved.Report.reject(issue{Code: "zip_unreadable", Message: fmt.Sprintf("读取 ZIP 失败 %v", sourceErr)})
		saved.Report.completeChecks(ticket)
	} else {
		file, err := os.Open(source)
		if err != nil {
			return saved, err
		}
		artifactDir := filepath.Join(dir, "artifact")
		if err := os.Mkdir(artifactDir, 0700); err != nil {
			file.Close()
			return saved, err
		}
		destination := filepath.Join(artifactDir, filepath.Base(source))
		if err := errors.Join(copyFile(destination, file), file.Close()); err != nil {
			return saved, err
		}
		saved.Report = verifyZIP(ticket, destination)
	}
	if err := writeJSON(filepath.Join(dir, verificationFilename), saved.Report); err != nil {
		return saved, err
	}
	receipt := makeReceipt(ticket, saved.Report, "verify")
	receipt.SourceZIPPath = source
	if err := recordArtifact(&receipt, dir); err != nil {
		return saved, err
	}
	return saved, saveReceipt(dir, receipt)
}

func listHistory(resultsPath string, filter historyFilter) ([]historyEntry, error) {
	var from, to time.Time
	var err error
	if filter.FromDate != "" {
		from, err = time.ParseInLocation("2006-01-02", filter.FromDate, time.Local)
		if err != nil {
			return nil, fmt.Errorf("起始日期必须使用 YYYY-MM-DD：%w", err)
		}
	}
	if filter.ToDate != "" {
		to, err = time.ParseInLocation("2006-01-02", filter.ToDate, time.Local)
		if err != nil {
			return nil, fmt.Errorf("结束日期必须使用 YYYY-MM-DD：%w", err)
		}
		to = to.AddDate(0, 0, 1)
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return nil, errors.New("起始日期不能晚于结束日期")
	}
	if filter.Status != "" && filter.Status != statusPassed && filter.Status != statusRejected {
		return nil, errors.New("历史状态必须为 passed 或 rejected")
	}
	results, err := filepath.Abs(resultsPath)
	if err != nil {
		return nil, err
	}
	children, err := os.ReadDir(results)
	if errors.Is(err, fs.ErrNotExist) {
		return []historyEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries := []historyEntry{}
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		dir := filepath.Join(results, child.Name())
		entry, found, err := readHistoryEntry(dir)
		if err != nil {
			return nil, fmt.Errorf("读取历史记录 %q 失败 %w", dir, err)
		}
		if !found || (filter.OrderID != "" && entry.OrderID != filter.OrderID) ||
			(filter.Version != "" && entry.FromVersion != filter.Version && entry.ToVersion != filter.Version) ||
			(filter.Status != "" && entry.Status != filter.Status) {
			continue
		}
		if !from.IsZero() || !to.IsZero() {
			if entry.CreatedAt == "" {
				continue
			}
			created, err := time.Parse(time.RFC3339Nano, entry.CreatedAt)
			if err != nil {
				return nil, err
			}
			if (!from.IsZero() && created.Before(from)) || (!to.IsZero() && !created.Before(to)) {
				continue
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt == entries[j].CreatedAt {
			return entries[i].Directory < entries[j].Directory
		}
		return entries[i].CreatedAt > entries[j].CreatedAt
	})
	return entries, nil
}

func readHistoryEntry(dir string) (historyEntry, bool, error) {
	entry := historyEntry{Directory: dir}
	_, err := os.Stat(filepath.Join(dir, "receipt.json"))
	if err == nil {
		receipt, err := readJSON[receiptRecord](filepath.Join(dir, "receipt.json"))
		if err != nil {
			return entry, false, err
		}
		if receipt.SchemaVersion != 1 || receipt.Order.OrderID == "" ||
			(receipt.Status != statusPassed && receipt.Status != statusRejected) {
			return entry, false, errors.New("历史记录版本、工单或状态无效")
		}
		if _, err := time.Parse(time.RFC3339Nano, receipt.CreatedAt); err != nil {
			return entry, false, fmt.Errorf("历史记录时间无效：%w", err)
		}
		entry.OrderID = receipt.Order.OrderID
		entry.FromVersion = receipt.Order.FromVersion
		entry.ToVersion = receipt.Order.ToVersion
		entry.Status = receipt.Status
		entry.Kind = receipt.Kind
		entry.CreatedAt = receipt.CreatedAt
		entry.ZIPPath = receipt.ZIPPath
		return entry, true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return entry, false, err
	}
	if _, err := os.Stat(filepath.Join(dir, verificationFilename)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return entry, false, nil
		}
		return entry, false, err
	}
	ticket, _, err := loadOrder(filepath.Join(dir, orderFilename))
	if err != nil {
		return entry, false, err
	}
	report, err := readJSON[verification](filepath.Join(dir, verificationFilename))
	if err != nil {
		return entry, false, err
	}
	if report.OrderID != ticket.OrderID || (report.Status != statusPassed && report.Status != statusRejected) {
		return entry, false, errors.New("旧记录验收结果与工单不符")
	}
	entry.OrderID = ticket.OrderID
	entry.FromVersion = ticket.FromVersion
	entry.ToVersion = ticket.ToVersion
	entry.Status = report.Status
	entry.Kind = "legacy"
	entry.TimeNotice = "旧记录未保存创建时间，无法参与日期筛选"
	entry.ZIPPath = report.ZIPPath
	if _, err := os.Stat(filepath.Join(dir, executionFilename)); err == nil {
		execution, err := readJSON[executionRecord](filepath.Join(dir, executionFilename))
		if err != nil {
			return entry, false, err
		}
		if !execution.passed() {
			entry.Status = statusRejected
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return entry, false, err
	}
	return entry, true, nil
}
