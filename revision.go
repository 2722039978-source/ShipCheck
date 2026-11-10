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
	"reflect"
	"sort"
)

type contentFingerprint struct {
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

type contentChange struct {
	Path   string               `json:"path"`
	Change string               `json:"change"`
	Before []contentFingerprint `json:"before,omitempty"`
	After  []contentFingerprint `json:"after,omitempty"`
}

type attemptComparison struct {
	Resolved               []issue         `json:"resolved"`
	Persisting             []issue         `json:"persisting"`
	Added                  []issue         `json:"added"`
	Unverified             []issue         `json:"unverified"`
	InputChanges           []contentChange `json:"input_changes"`
	ZIPChanges             []contentChange `json:"zip_changes"`
	ZIPComparisonAvailable bool            `json:"zip_comparison_available"`
	ZIPComparisonNotice    string          `json:"zip_comparison_notice,omitempty"`
}

func (comparison attemptComparison) writeTo(out io.Writer) {
	for _, group := range []struct {
		name   string
		issues []issue
	}{
		{"已解决问题", comparison.Resolved},
		{"持续存在问题", comparison.Persisting},
		{"新增问题", comparison.Added},
		{"无法确认的问题", comparison.Unverified},
	} {
		fmt.Fprintf(out, "%s：%d\n", group.name, len(group.issues))
		for _, problem := range group.issues {
			fmt.Fprintf(out, "- [%s] %s\n", problem.Code, problem.Message)
		}
	}
	for _, group := range []struct {
		name    string
		changes []contentChange
	}{{"输入内容变化", comparison.InputChanges}, {"ZIP 内容变化", comparison.ZIPChanges}} {
		fmt.Fprintf(out, "%s：%d\n", group.name, len(group.changes))
		for _, change := range group.changes {
			fmt.Fprintf(out, "- %s %s\n", change.Change, change.Path)
		}
	}
	if comparison.ZIPComparisonNotice != "" {
		fmt.Fprintln(out, comparison.ZIPComparisonNotice)
	}
}

func writeIssueComparison(out io.Writer, original, current verification) {
	comparison := compareIssues(original, current)
	comparison.writeTo(out)
}

type issueIdentity struct {
	Code     string
	Path     string
	Field    string
	Expected string
}

func identityOf(problem issue) issueIdentity {
	return issueIdentity{problem.Code, problem.Path, problem.Field, problem.Expected}
}

func compareIssues(original, current verification) attemptComparison {
	comparison := attemptComparison{
		Resolved: []issue{}, Persisting: []issue{}, Added: []issue{}, Unverified: []issue{},
		InputChanges: []contentChange{}, ZIPChanges: []contentChange{},
	}
	previous := make(map[issueIdentity]int)
	remaining := make(map[issueIdentity]int)
	for _, problem := range original.Issues {
		previous[identityOf(problem)]++
	}
	for _, problem := range current.Issues {
		key := identityOf(problem)
		if previous[key] > 0 {
			comparison.Persisting = append(comparison.Persisting, problem)
			previous[key]--
			remaining[key]++
		} else {
			comparison.Added = append(comparison.Added, problem)
		}
	}
	for _, problem := range original.Issues {
		key := identityOf(problem)
		if remaining[key] > 0 {
			remaining[key]--
			continue
		}
		if issueCheckUnavailable(problem, current) {
			comparison.Unverified = append(comparison.Unverified, problem)
		} else {
			comparison.Resolved = append(comparison.Resolved, problem)
		}
	}
	return comparison
}

func issueCheckUnavailable(problem issue, current verification) bool {
	if problem.Code == "packaging_command_failed" {
		return false
	}
	if !current.ArchiveReadable {
		return true
	}
	for _, other := range current.Issues {
		if other.Path == problem.Path && (other.Code == issueFileRead || other.Code == "invalid_release" || other.Code == "duplicate_path" || other.Code == issueMissingFile) {
			return true
		}
	}
	return false
}

func executionIssues(report verification, execution executionRecord) verification {
	copy := report
	copy.Issues = append([]issue{}, report.Issues...)
	if !execution.passed() {
		code := "未取得退出码"
		if execution.ExitCode != nil {
			code = fmt.Sprint(*execution.ExitCode)
		}
		copy.Issues = append(copy.Issues, issue{Code: "packaging_command_failed", Expected: "0", Actual: code, Message: "打包命令失败，退出码 " + code + " " + execution.Error})
	}
	return copy
}

func revisePackaging(evidencePath, stagingOverride, resultsPath string, commandOverride *commandSpec) (attempt, verification, error) {
	return derivePackaging(evidencePath, stagingOverride, resultsPath, commandOverride, "revise")
}

func derivePackaging(evidencePath, stagingOverride, resultsPath string, commandOverride *commandSpec, kind string) (result attempt, original verification, err error) {
	evidence, err := canonicalDirectory(evidencePath)
	if err != nil {
		return result, original, fmt.Errorf("打开证据目录失败 %w", err)
	}
	ticket, orderData, err := loadOrder(filepath.Join(evidence, orderFilename))
	if err != nil {
		return result, original, err
	}
	record, err := readJSON[executionRecord](filepath.Join(evidence, executionFilename))
	if err != nil {
		return result, original, err
	}
	original, err = readJSON[verification](filepath.Join(evidence, verificationFilename))
	if err != nil {
		return result, original, err
	}
	if record.SchemaVersion != evidenceVersion || record.Command.Tool == "" ||
		(record.Command.Executable != "" && !filepath.IsAbs(record.Command.Executable)) ||
		(record.Command.WorkspaceTool != "" && !validRelativePath(record.Command.WorkspaceTool)) {
		return result, original, errors.New("证据中的打包命令或记录版本无效")
	}
	if original.OrderID != ticket.OrderID || (original.Status != statusPassed && original.Status != statusRejected) {
		return result, original, errors.New("证据中的验收结果与工单不符")
	}
	if record.passed() && original.Status == statusPassed {
		return result, original, errors.New("证据目录记录的打包已通过")
	}
	output, err := localOutputPath(record.OutputPath)
	if err != nil {
		return result, original, err
	}
	originalInput := filepath.Join(evidence, inputDirectory)
	source := originalInput
	if stagingOverride != "" {
		source, err = canonicalDirectory(stagingOverride)
		if err != nil {
			return result, original, err
		}
	}
	oldInputs, err := inputInventory(originalInput)
	if err != nil {
		return result, original, fmt.Errorf("读取原输入快照失败 %w", err)
	}
	command := record.Command
	if commandOverride != nil {
		command = *commandOverride
		if command.Tool == "" {
			return result, original, errors.New("修正命令必须指定工具")
		}
	}
	dir, err := newResultDirectory(resultsPath, evidence, kind+"-")
	if err != nil {
		return result, original, err
	}
	result.Directory = dir
	defer func() {
		if err != nil {
			err = fmt.Errorf("结果目录 %q\n%w", dir, err)
		}
	}()
	if relative, err := filepath.Rel(source, dir); err == nil && filepath.IsLocal(relative) {
		return result, original, errors.New("修正结果目录必须位于输入目录之外")
	}
	if err := os.WriteFile(filepath.Join(dir, orderFilename), orderData, 0600); err != nil {
		return result, original, err
	}
	snapshot := filepath.Join(dir, inputDirectory)
	if err := os.Mkdir(snapshot, 0700); err != nil {
		return result, original, err
	}
	if _, err := copyInputs(source, snapshot); err != nil {
		return result, original, err
	}
	newInputs, err := inputInventory(snapshot)
	if err != nil {
		return result, original, err
	}
	result, err = performAttempt(ticket, snapshot, dir, output, command)
	if err != nil {
		return result, original, err
	}
	comparison := compareIssues(executionIssues(original, record), executionIssues(result.Verification, result.Execution))
	comparison.InputChanges = compareContents(oldInputs, newInputs)
	originalZIP := filepath.Join(evidence, "artifact", filepath.Base(original.ZIPPath))
	oldZIP, oldErr := zipInventory(originalZIP)
	newZIP, newErr := zipInventory(result.Verification.ZIPPath)
	if oldErr == nil && newErr == nil {
		comparison.ZIPComparisonAvailable = true
		comparison.ZIPChanges = compareContents(oldZIP, newZIP)
	} else {
		comparison.ZIPComparisonNotice = "ZIP 内容无法完整比较"
		if oldErr != nil {
			comparison.ZIPComparisonNotice += "；原 ZIP：" + oldErr.Error()
		}
		if newErr != nil {
			comparison.ZIPComparisonNotice += "；本次 ZIP：" + newErr.Error()
		}
	}
	err = finishAttempt(&result, ticket, kind, evidence, &comparison)
	return result, original, err
}

func inputInventory(directory string) (map[string][]contentFingerprint, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := make(map[string][]contentFingerprint)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if entry.IsDir() {
			files[name] = []contentFingerprint{{Kind: "directory"}}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("输入 %q 必须为普通文件或目录", name)
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return err
		}
		file, err := root.Open(local)
		if err != nil {
			return err
		}
		fingerprint, hashErr := fingerprintContent(file, "file")
		if err := errors.Join(hashErr, file.Close()); err != nil {
			return err
		}
		files[name] = []contentFingerprint{fingerprint}
		return nil
	})
	return files, err
}

func fingerprintContent(reader io.Reader, kind string) (contentFingerprint, error) {
	digest := sha256.New()
	size, err := io.Copy(digest, reader)
	if err != nil {
		return contentFingerprint{}, err
	}
	return contentFingerprint{Kind: kind, Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func zipInventory(path string) (map[string][]contentFingerprint, error) {
	archive, err := openZIP(path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	files := make(map[string][]contentFingerprint)
	for _, entry := range archive.File {
		kind := "file"
		if entry.Mode().IsDir() {
			kind = "directory"
		} else if !entry.Mode().IsRegular() {
			kind = entry.Mode().String()
		}
		content, err := entry.Open()
		if err != nil {
			return nil, err
		}
		fingerprint, hashErr := fingerprintContent(content, kind)
		if err := errors.Join(hashErr, content.Close()); err != nil {
			return nil, err
		}
		files[entry.Name] = append(files[entry.Name], fingerprint)
	}
	return files, nil
}

func compareContents(original, current map[string][]contentFingerprint) []contentChange {
	keys := make(map[string]struct{})
	for name := range original {
		keys[name] = struct{}{}
	}
	for name := range current {
		keys[name] = struct{}{}
	}
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)
	changes := []contentChange{}
	for _, name := range names {
		before, hadBefore := original[name]
		after, hasAfter := current[name]
		if reflect.DeepEqual(before, after) {
			continue
		}
		change := "modified"
		if !hadBefore {
			change = "added"
		} else if !hasAfter {
			change = "removed"
		}
		changes = append(changes, contentChange{Path: name, Change: change, Before: before, After: after})
	}
	return changes
}
