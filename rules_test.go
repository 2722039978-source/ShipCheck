package main

import (
	"encoding/json"
	"testing"
)

func rulesStructuredRelease(t *testing.T, components, migrations any) string {
	t.Helper()
	value := map[string]any{"from_version": "1.0", "to_version": "2.0", "platform": "windows-amd64"}
	if components != nil {
		value["components"] = components
	}
	if migrations != nil {
		value["migrations"] = migrations
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRulesComponentsAndDependencies(t *testing.T) {
	components := []any{
		map[string]any{"name": "app", "version": "2.0", "dependencies": []any{map[string]any{"name": "runtime", "allowed_versions": []string{"3.0", "3.1"}}}},
		map[string]any{"name": "runtime", "version": "3.1"},
	}
	ticket := rulesOrder(t, map[string]any{"components": components})
	cases := []struct {
		name       string
		components []any
		code       string
	}{
		{"valid", []any{map[string]any{"name": "app", "version": "2.0"}, map[string]any{"name": "runtime", "version": "3.1"}}, ""},
		{"version", []any{map[string]any{"name": "app", "version": "1.9"}, map[string]any{"name": "runtime", "version": "3.1"}}, "component_version_mismatch"},
		{"dependency", []any{map[string]any{"name": "app", "version": "2.0"}, map[string]any{"name": "runtime", "version": "4.0"}}, "dependency_conflict"},
		{"missing", []any{map[string]any{"name": "app", "version": "2.0"}}, "missing_component"},
		{"duplicate", []any{map[string]any{"name": "app", "version": "2.0"}, map[string]any{"name": "app", "version": "2.0"}}, "invalid_components"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			release := rulesStructuredRelease(t, item.components, nil)
			report := verifyZIP(ticket, rulesZIP(t, []rulesZIPEntry{{releasePath, release}, {"payload/app.bin", "payload"}}))
			if item.code != "" {
				rulesAssertIssue(t, report, item.code, releasePath)
			} else if report.Status != statusPassed {
				t.Fatalf("组件检查失败 %+v", report)
			}
		})
	}
}

func TestRulesMigrations(t *testing.T) {
	first := map[string]any{"id": "step-1", "from_version": "1.0", "to_version": "1.5", "files": []string{"migrations/001.sql"}}
	second := map[string]any{"id": "step-2", "from_version": "1.5", "to_version": "2.0", "files": []string{"migrations/002.sql"}}
	ticket := rulesOrder(t, map[string]any{"migrations": []any{first, second}, "allow_extra_files": false})
	cases := []struct {
		name       string
		migrations []any
		files      []rulesZIPEntry
		code       string
	}{
		{"valid", []any{first, second}, []rulesZIPEntry{{"migrations/001.sql", "SELECT 1;"}, {"migrations/002.sql", "SELECT 2;"}}, ""},
		{"missing-step", []any{second}, []rulesZIPEntry{{"migrations/001.sql", "SELECT 1;"}, {"migrations/002.sql", "SELECT 2;"}}, "migration_mismatch"},
		{"wrong-order", []any{second, first}, []rulesZIPEntry{{"migrations/001.sql", "SELECT 1;"}, {"migrations/002.sql", "SELECT 2;"}}, "migration_mismatch"},
		{"missing-file", []any{first, second}, []rulesZIPEntry{{"migrations/001.sql", "SELECT 1;"}}, issueMissingFile},
		{"changed-file", []any{first, map[string]any{"id": "step-2", "from_version": "1.5", "to_version": "2.0", "files": []string{"migrations/003.sql"}}}, []rulesZIPEntry{{"migrations/001.sql", "SELECT 1;"}, {"migrations/002.sql", "SELECT 2;"}}, "migration_mismatch"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			release := rulesStructuredRelease(t, nil, item.migrations)
			entries := append([]rulesZIPEntry{{releasePath, release}, {"payload/app.bin", "payload"}}, item.files...)
			report := verifyZIP(ticket, rulesZIP(t, entries))
			if item.code != "" {
				rulesAssertIssue(t, report, item.code, "")
			} else if report.Status != statusPassed {
				t.Fatalf("完整迁移验收失败 %+v", report)
			}
		})
	}
}
