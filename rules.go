package main

import (
	"archive/zip"
	"encoding/hex"
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type fileRule struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256,omitempty"`
	Size       *int64 `json:"size,omitempty"`
	AllowEmpty *bool  `json:"allow_empty,omitempty"`
}

type componentRequirement struct {
	Name         string                  `json:"name"`
	Version      string                  `json:"version"`
	Dependencies []dependencyRequirement `json:"dependencies,omitempty"`
}

type dependencyRequirement struct {
	Name            string   `json:"name"`
	AllowedVersions []string `json:"allowed_versions"`
}

type componentVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type migrationStep struct {
	ID          string   `json:"id"`
	FromVersion string   `json:"from_version"`
	ToVersion   string   `json:"to_version"`
	Files       []string `json:"files"`
}

type releaseDeclaration struct {
	releaseMetadata
	Components []componentVersion `json:"components,omitempty"`
	Migrations []migrationStep    `json:"migrations,omitempty"`
}

type verifiedFile struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256,omitempty"`
	Size     int64  `json:"size"`
	Readable bool   `json:"readable"`
}

type verificationCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Scope  string `json:"scope"`
}

func validateOrder(ticket order) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"order_id", ticket.OrderID},
		{"from_version", ticket.FromVersion},
		{"to_version", ticket.ToVersion},
		{"platform", ticket.Platform},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("工单的 %s 必须为非空字符串", field.name)
		}
	}
	if ticket.RequiredFiles == nil {
		return fmt.Errorf("工单的 required_files 必须为数组")
	}
	if err := validatePathList(ticket.RequiredFiles, "required_files"); err != nil {
		return err
	}
	fileNames := make([]string, 0, len(ticket.Files))
	for _, rule := range ticket.Files {
		fileNames = append(fileNames, rule.Path)
		if rule.SHA256 != "" && !validSHA256(rule.SHA256) {
			return fmt.Errorf("文件 %q 的 sha256 必须包含 64 个十六进制字符", rule.Path)
		}
		if rule.Size != nil && *rule.Size < 0 {
			return fmt.Errorf("文件 %q 的 size 必须为非负整数", rule.Path)
		}
		if rule.Size != nil && *rule.Size == 0 && rule.AllowEmpty != nil && !*rule.AllowEmpty {
			return fmt.Errorf("文件 %q 的 size 与 allow_empty 相互冲突", rule.Path)
		}
	}
	if err := validatePathList(fileNames, "files"); err != nil {
		return err
	}
	for _, directory := range ticket.AllowedDirectories {
		if directory != "." && !validRelativePath(directory) {
			return fmt.Errorf("allowed_directories 包含无效目录 %q", directory)
		}
	}
	for _, pattern := range ticket.ForbiddenPatterns {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("forbidden_patterns 必须使用非空规则")
		}
		if _, err := path.Match(pattern, releasePath); err != nil {
			return fmt.Errorf("无效的禁止路径规则 %q：%w", pattern, err)
		}
	}
	components := make(map[string]struct{}, len(ticket.Components))
	for _, component := range ticket.Components {
		if strings.TrimSpace(component.Name) == "" || strings.TrimSpace(component.Version) == "" {
			return fmt.Errorf("组件的 name 和 version 必须为非空字符串")
		}
		if _, exists := components[component.Name]; exists {
			return fmt.Errorf("重复声明组件 %q", component.Name)
		}
		components[component.Name] = struct{}{}
	}
	for _, component := range ticket.Components {
		dependencies := make(map[string]struct{})
		for _, dependency := range component.Dependencies {
			if _, exists := components[dependency.Name]; !exists {
				return fmt.Errorf("组件 %q 的依赖 %q 必须在 components 中声明", component.Name, dependency.Name)
			}
			if dependency.Name == component.Name {
				return fmt.Errorf("组件 %q 不能依赖自身", component.Name)
			}
			if _, exists := dependencies[dependency.Name]; exists {
				return fmt.Errorf("组件 %q 重复声明依赖 %q", component.Name, dependency.Name)
			}
			dependencies[dependency.Name] = struct{}{}
			if len(dependency.AllowedVersions) == 0 {
				return fmt.Errorf("组件 %q 的依赖 %q 必须提供 allowed_versions", component.Name, dependency.Name)
			}
			versions := make(map[string]struct{})
			for _, version := range dependency.AllowedVersions {
				if strings.TrimSpace(version) == "" {
					return fmt.Errorf("依赖 %q 的允许版本必须为非空字符串", dependency.Name)
				}
				if _, exists := versions[version]; exists {
					return fmt.Errorf("依赖 %q 重复声明允许版本 %q", dependency.Name, version)
				}
				versions[version] = struct{}{}
			}
		}
	}
	if ticket.Migrations != nil {
		if err := validateMigrationChain(ticket.Migrations, ticket.FromVersion, ticket.ToVersion); err != nil {
			return err
		}
	}
	if ticket.Template != nil {
		if strings.TrimSpace(ticket.Template.Name) == "" || strings.TrimSpace(ticket.Template.Version) == "" || !validSHA256(ticket.Template.SHA256) {
			return fmt.Errorf("模板来源必须包含 name、version 和有效的 sha256")
		}
	}
	return validateCombinedPaths(requiredPaths(ticket))
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validatePathList(names []string, field string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validRelativePath(name) {
			return fmt.Errorf("%s 包含无效的包内相对路径 %q", field, name)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s 重复声明路径 %q", field, name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateCombinedPaths(names []string) error {
	seen := make(map[string]string, len(names))
	for _, name := range names {
		key := strings.ToLower(name)
		if previous, exists := seen[key]; exists && previous != name {
			return fmt.Errorf("工单文件 %q 与 %q 的大小写冲突", previous, name)
		}
		seen[key] = name
	}
	for _, name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := seen[strings.ToLower(parent)]; exists {
				return fmt.Errorf("工单路径 %q 同时用作文件与目录", parent)
			}
		}
	}
	return nil
}

func requiredPaths(ticket order) []string {
	names := map[string]struct{}{releasePath: {}}
	for _, name := range ticket.RequiredFiles {
		names[name] = struct{}{}
	}
	for _, rule := range ticket.Files {
		names[rule.Path] = struct{}{}
	}
	for _, step := range ticket.Migrations {
		for _, name := range step.Files {
			names[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func checkArchivePolicy(report *verification, ticket order, entries []*zip.File) {
	required := make(map[string]struct{})
	for _, name := range requiredPaths(ticket) {
		required[name] = struct{}{}
	}
	seen := make(map[string]string)
	files := make(map[string]bool)
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name, "/")
		key := strings.ToLower(name)
		if !validRelativePath(name) {
			report.reject(issue{Code: "invalid_path", Path: entry.Name, Message: fmt.Sprintf("ZIP 包含无效相对路径 %q", entry.Name)})
		}
		if previous, exists := seen[key]; exists {
			report.reject(issue{Code: "duplicate_path", Path: entry.Name, Message: fmt.Sprintf("ZIP 路径 %q 与 %q 重复或大小写冲突", entry.Name, previous)})
		}
		seen[key] = entry.Name
		if !entry.FileInfo().IsDir() {
			files[key] = true
		}
		if ticket.AllowedDirectories != nil && name != releasePath && !allowedArchivePath(name, entry.FileInfo().IsDir(), ticket.AllowedDirectories) {
			report.reject(issue{Code: "directory_not_allowed", Path: entry.Name, Message: fmt.Sprintf("路径 %q 位于允许目录之外", entry.Name)})
		}
		for _, pattern := range ticket.ForbiddenPatterns {
			if forbiddenArchivePath(name, pattern) {
				report.reject(issue{Code: "forbidden_path", Path: entry.Name, Expected: pattern, Message: fmt.Sprintf("路径 %q 符合禁止规则 %q", entry.Name, pattern)})
			}
		}
		if ticket.AllowExtraFiles != nil && !*ticket.AllowExtraFiles && !entry.FileInfo().IsDir() {
			if _, exists := required[entry.Name]; !exists {
				report.reject(issue{Code: "extra_file", Path: entry.Name, Message: fmt.Sprintf("ZIP 包含清单之外的文件 %q", entry.Name)})
			}
		}
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name, "/")
		for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if files[strings.ToLower(parent)] {
				report.reject(issue{Code: "path_conflict", Path: entry.Name, Message: fmt.Sprintf("路径 %q 的父目录 %q 同时用作文件", entry.Name, parent)})
			}
		}
	}
}

func allowedArchivePath(name string, directory bool, allowed []string) bool {
	for _, parent := range allowed {
		if parent == "." {
			if !directory && path.Dir(name) == "." {
				return true
			}
			continue
		}
		if strings.HasPrefix(name, parent+"/") || (directory && (name == parent || strings.HasPrefix(parent, name+"/"))) {
			return true
		}
	}
	return false
}

func forbiddenArchivePath(name, pattern string) bool {
	for candidate := name; candidate != "." && candidate != "/"; candidate = path.Dir(candidate) {
		matches, err := path.Match(strings.ToLower(pattern), strings.ToLower(candidate))
		if err != nil {
			return false
		}
		if matches {
			return true
		}
	}
	return false
}

func checkFileRules(report *verification, ticket order, actual verifiedFile) {
	for _, rule := range ticket.Files {
		if rule.Path != actual.Path {
			continue
		}
		if rule.SHA256 != "" && !strings.EqualFold(rule.SHA256, actual.SHA256) {
			report.reject(issue{Code: "sha256_mismatch", Path: actual.Path, Field: "sha256", Expected: rule.SHA256, Actual: actual.SHA256, Message: fmt.Sprintf("文件 %q 的 SHA-256 不匹配", actual.Path)})
		}
		if rule.Size != nil && *rule.Size != actual.Size {
			report.reject(issue{Code: "size_mismatch", Path: actual.Path, Field: "size", Expected: strconv.FormatInt(*rule.Size, 10), Actual: strconv.FormatInt(actual.Size, 10), Message: fmt.Sprintf("文件 %q 的字节数不匹配：要求 %d，实际 %d", actual.Path, *rule.Size, actual.Size)})
		}
		if rule.AllowEmpty != nil && !*rule.AllowEmpty && actual.Size == 0 {
			report.reject(issue{Code: "empty_file", Path: actual.Path, Message: fmt.Sprintf("文件 %q 不能使用空内容", actual.Path)})
		}
	}
}

func checkComponents(report *verification, ticket order, actual []componentVersion) {
	if ticket.Components == nil {
		return
	}
	components := make(map[string]string)
	for _, component := range actual {
		if strings.TrimSpace(component.Name) == "" || strings.TrimSpace(component.Version) == "" {
			report.reject(issue{Code: "invalid_components", Path: releasePath, Field: "components", Message: "release.json 的组件必须包含非空 name 和 version"})
			continue
		}
		if _, exists := components[component.Name]; exists {
			report.reject(issue{Code: "invalid_components", Path: releasePath, Field: "components", Message: fmt.Sprintf("release.json 重复声明组件 %q", component.Name)})
		}
		components[component.Name] = component.Version
	}
	expected := make(map[string]struct{})
	for _, component := range ticket.Components {
		expected[component.Name] = struct{}{}
		version, exists := components[component.Name]
		if !exists {
			report.reject(issue{Code: "missing_component", Path: releasePath, Field: "components." + component.Name, Expected: component.Version, Message: fmt.Sprintf("release.json 缺少组件 %q", component.Name)})
		} else if version != component.Version {
			report.reject(issue{Code: "component_version_mismatch", Path: releasePath, Field: "components." + component.Name, Expected: component.Version, Actual: version, Message: fmt.Sprintf("组件 %q 的版本不匹配：要求 %q，实际 %q", component.Name, component.Version, version)})
		}
		for _, dependency := range component.Dependencies {
			version, exists := components[dependency.Name]
			if !exists || !slices.Contains(dependency.AllowedVersions, version) {
				report.reject(issue{Code: "dependency_conflict", Path: releasePath, Field: "components." + component.Name + ".dependencies." + dependency.Name, Expected: strings.Join(dependency.AllowedVersions, ", "), Actual: version, Message: fmt.Sprintf("组件 %q 的依赖 %q 不满足允许版本 %v，实际 %q", component.Name, dependency.Name, dependency.AllowedVersions, version)})
			}
		}
	}
	for _, component := range actual {
		if _, exists := expected[component.Name]; !exists {
			report.reject(issue{Code: "unexpected_component", Path: releasePath, Field: "components." + component.Name, Actual: component.Version, Message: fmt.Sprintf("release.json 声明了工单之外的组件 %q", component.Name)})
		}
	}
}

func validateMigrationChain(steps []migrationStep, from, to string) error {
	current := from
	ids := make(map[string]struct{})
	versions := map[string]struct{}{from: {}}
	for _, step := range steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.FromVersion) == "" || strings.TrimSpace(step.ToVersion) == "" {
			return fmt.Errorf("迁移步骤必须包含非空 id、from_version 和 to_version")
		}
		if _, exists := ids[step.ID]; exists {
			return fmt.Errorf("重复声明迁移步骤 %q", step.ID)
		}
		ids[step.ID] = struct{}{}
		if step.FromVersion != current {
			return fmt.Errorf("迁移步骤 %q 必须从版本 %q 开始，声明版本为 %q", step.ID, current, step.FromVersion)
		}
		if _, exists := versions[step.ToVersion]; exists {
			return fmt.Errorf("迁移步骤 %q 重复到达版本 %q", step.ID, step.ToVersion)
		}
		versions[step.ToVersion] = struct{}{}
		if len(step.Files) == 0 {
			return fmt.Errorf("迁移步骤 %q 必须声明关联文件", step.ID)
		}
		if err := validatePathList(step.Files, "migrations.files"); err != nil {
			return err
		}
		current = step.ToVersion
	}
	if current != to {
		return fmt.Errorf("迁移步骤结束于版本 %q，要求目标版本 %q", current, to)
	}
	return nil
}

func checkMigrations(report *verification, ticket order, actual []migrationStep) {
	if ticket.Migrations == nil {
		return
	}
	if err := validateMigrationChain(actual, ticket.FromVersion, ticket.ToVersion); err != nil {
		report.reject(issue{Code: "migration_mismatch", Path: releasePath, Field: "migrations", Message: err.Error()})
	}
	if len(actual) != len(ticket.Migrations) {
		report.reject(issue{Code: "migration_mismatch", Path: releasePath, Field: "migrations", Expected: strconv.Itoa(len(ticket.Migrations)), Actual: strconv.Itoa(len(actual)), Message: "release.json 的迁移步骤数量与工单不匹配"})
	}
	for index, expected := range ticket.Migrations {
		if index >= len(actual) {
			break
		}
		step := actual[index]
		if expected.ID != step.ID || expected.FromVersion != step.FromVersion || expected.ToVersion != step.ToVersion || !slices.Equal(expected.Files, step.Files) {
			report.reject(issue{Code: "migration_mismatch", Path: releasePath, Field: fmt.Sprintf("migrations[%d]", index), Expected: expected.ID + ":" + expected.FromVersion + "->" + expected.ToVersion + ":" + strings.Join(expected.Files, ","), Actual: step.ID + ":" + step.FromVersion + "->" + step.ToVersion + ":" + strings.Join(step.Files, ","), Message: fmt.Sprintf("第 %d 个迁移步骤的标识、版本或关联文件与工单不匹配", index+1)})
		}
	}
}

func (report *verification) completeChecks(ticket order) {
	checks := []struct {
		name      string
		scope     string
		requested bool
		codes     []string
	}{
		{"archive", "读取 ZIP，检查全部路径、重复名称、目录、禁止路径与清单之外文件", true, []string{"zip_unreadable", "output_unavailable", "invalid_path", "duplicate_path", "path_conflict", "directory_not_allowed", "forbidden_path", "extra_file"}},
		{"files", "完整读取全部普通文件并记录 SHA-256 和字节数；按工单核对必需文件与内容规则", true, []string{issueMissingFile, issueFileRead, "sha256_mismatch", "size_mismatch", "empty_file"}},
		{"metadata", "核对 release.json 的起始版本、目标版本与平台", true, []string{"invalid_release", "metadata_mismatch"}},
		{"components", "核对声明的组件版本与依赖允许版本列表", ticket.Components != nil, []string{"invalid_components", "missing_component", "component_version_mismatch", "dependency_conflict", "unexpected_component"}},
		{"migrations", "核对迁移声明、完整顺序、版本连续性和关联文件；目标环境负责试装验证", ticket.Migrations != nil, []string{"migration_mismatch"}},
	}
	for _, check := range checks {
		status := statusPassed
		if !check.requested {
			status = "not_requested"
		} else if !report.ArchiveReadable {
			status = "not_run"
		}
		for _, problem := range report.Issues {
			if slices.Contains(check.codes, problem.Code) {
				status = statusRejected
			}
			if check.requested && check.name != "archive" && check.name != "files" && problem.Path == releasePath && (problem.Code == issueMissingFile || problem.Code == issueFileRead || problem.Code == "invalid_release") {
				status = statusRejected
			}
			if check.name == "migrations" && check.requested && (problem.Code == issueMissingFile || problem.Code == issueFileRead) {
				for _, step := range ticket.Migrations {
					if slices.Contains(step.Files, problem.Path) {
						status = statusRejected
					}
				}
			}
		}
		report.Checks = append(report.Checks, verificationCheck{Name: check.name, Status: status, Scope: check.scope})
	}
}
