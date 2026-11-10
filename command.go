package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const evidenceVersion = 1

type commandSpec struct {
	Tool          string   `json:"tool"`
	Executable    string   `json:"executable,omitempty"`
	WorkspaceTool string   `json:"workspace_tool,omitempty"`
	Args          []string `json:"args"`
}

type executionRecord struct {
	SchemaVersion int         `json:"schema_version"`
	OutputPath    string      `json:"output_path"`
	WorkingDir    string      `json:"working_directory"`
	Command       commandSpec `json:"command"`
	ExitCode      *int        `json:"exit_code"`
	Error         string      `json:"error,omitempty"`
}

func (record executionRecord) passed() bool {
	return record.ExitCode != nil && *record.ExitCode == 0 && record.Error == ""
}

func (record executionRecord) writeTo(out io.Writer) {
	if record.ExitCode == nil {
		fmt.Fprintln(out, "打包命令未取得退出码")
	} else {
		fmt.Fprintln(out, "打包命令退出码", *record.ExitCode)
	}
	if record.Error != "" {
		fmt.Fprintln(out, "打包命令失败", record.Error)
	}
}

func (command commandSpec) resolve(workDir string) (commandSpec, error) {
	if command.WorkspaceTool != "" {
		local, err := filepath.Localize(command.WorkspaceTool)
		if err != nil {
			return command, err
		}
		command.Executable = filepath.Join(workDir, local)
		return command, nil
	}
	if command.Executable != "" {
		return command, nil
	}

	candidate := command.Tool
	inWorkspace := !filepath.IsAbs(candidate) && strings.ContainsAny(candidate, "/\\")
	if inWorkspace {
		if !filepath.IsLocal(candidate) {
			return command, fmt.Errorf("工具的相对路径 %q 必须指向工作目录内部", candidate)
		}
		candidate = filepath.Join(workDir, candidate)
	}
	executable, err := exec.LookPath(candidate)
	if err != nil {
		return command, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return command, err
	}
	if inWorkspace {
		relative, err := filepath.Rel(workDir, executable)
		if err != nil {
			return command, err
		}
		command.WorkspaceTool = filepath.ToSlash(relative)
	}
	command.Executable = executable
	return command, nil
}

func executeCommand(workDir, resultDir, output string, command commandSpec) (record executionRecord, err error) {
	record = executionRecord{
		SchemaVersion: evidenceVersion,
		OutputPath:    filepath.ToSlash(output),
		WorkingDir:    workDir,
		Command:       command,
	}
	stdout, err := os.OpenFile(filepath.Join(resultDir, "stdout.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return record, err
	}
	defer func() { err = errors.Join(err, stdout.Close()) }()
	stderr, err := os.OpenFile(filepath.Join(resultDir, "stderr.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return record, err
	}
	defer func() { err = errors.Join(err, stderr.Close()) }()

	resolved, resolveErr := command.resolve(workDir)
	if resolveErr != nil {
		record.Error = resolveErr.Error()
		return record, nil
	}
	record.Command = resolved
	process := exec.Command(resolved.Executable, resolved.Args...)
	configurePackagingProcess(process)
	process.Dir = workDir
	process.Stdout = stdout
	process.Stderr = stderr
	if runErr := process.Run(); runErr != nil {
		record.Error = runErr.Error()
	}
	if process.ProcessState != nil {
		code := process.ProcessState.ExitCode()
		record.ExitCode = &code
	}
	return record, nil
}
