package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	orderFilename        = "order.json"
	executionFilename    = "execution.json"
	verificationFilename = "verification.json"
	inputDirectory       = "input"
)

type attempt struct {
	Directory    string
	Execution    executionRecord
	Verification verification
	Receipt      receiptRecord
}

func (result attempt) passed() bool {
	return result.Execution.passed() && result.Verification.Status == statusPassed
}

func (result attempt) writeTo(out io.Writer) {
	result.Execution.writeTo(out)
	result.Verification.writeTo(out)
	if result.Receipt.Comparison != nil {
		result.Receipt.Comparison.writeTo(out)
	}
	if result.passed() {
		fmt.Fprintln(out, "交付包", result.Verification.ZIPPath)
	}
}

func runPackaging(orderPath, stagingPath, outputName, resultsPath string, command commandSpec) (result attempt, err error) {
	ticket, orderData, err := loadOrder(orderPath)
	if err != nil {
		return result, err
	}
	output, err := localOutputPath(outputName)
	if err != nil {
		return result, err
	}
	staging, err := canonicalDirectory(stagingPath)
	if err != nil {
		return result, fmt.Errorf("打开暂存目录失败 %w", err)
	}
	dir, err := newResultDirectory(resultsPath, staging, "run-")
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			err = fmt.Errorf("结果目录 %q\n%w", dir, err)
		}
	}()

	if err := os.WriteFile(filepath.Join(dir, orderFilename), orderData, 0600); err != nil {
		return result, err
	}
	snapshot := filepath.Join(dir, inputDirectory)
	if err := os.Mkdir(snapshot, 0700); err != nil {
		return result, err
	}
	if _, err := copyInputs(staging, snapshot); err != nil {
		return result, fmt.Errorf("保存打包前的输入失败 %w", err)
	}
	result, err = performAttempt(ticket, snapshot, dir, output, command)
	if err != nil {
		return result, err
	}
	err = finishAttempt(&result, ticket, "run", "", nil)
	return result, err
}

func replayPackaging(evidencePath, resultsPath string) (result attempt, original verification, err error) {
	return derivePackaging(evidencePath, "", resultsPath, nil, "replay")
}

func performAttempt(ticket order, snapshot, dir, output string, command commandSpec) (result attempt, err error) {
	result.Directory = dir
	workDir, err := os.MkdirTemp(filepath.Dir(dir), "work-")
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(workDir)) }()

	inputs, err := copyInputs(snapshot, workDir)
	if err != nil {
		return result, fmt.Errorf("恢复工作副本失败 %w", err)
	}
	workspace, err := os.OpenRoot(workDir)
	if err != nil {
		return result, err
	}
	defer workspace.Close()

	if _, err := workspace.Lstat(output); err == nil {
		return result, fmt.Errorf("输入中已存在预期输出路径 %q，请指定未使用的输出路径", filepath.ToSlash(output))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, fmt.Errorf("检查预期输出路径失败 %w", err)
	}

	result.Execution, err = executeCommand(workDir, dir, output, command)
	if err != nil {
		return result, fmt.Errorf("记录打包日志失败 %w", err)
	}
	if err := writeJSON(filepath.Join(dir, executionFilename), result.Execution); err != nil {
		return result, err
	}
	zipPath, err := captureZIP(workspace, output, dir)
	if err == nil {
		result.Verification = verifyZIP(ticket, zipPath)
	} else {
		result.Verification = newVerification(ticket, filepath.Join(workDir, output))
		result.Verification.reject(issue{Code: "output_unavailable", Message: err.Error()})
		result.Verification.completeChecks(ticket)
	}
	result.Verification.diagnoseMissing(inputs)
	if err := writeJSON(filepath.Join(dir, verificationFilename), result.Verification); err != nil {
		return result, err
	}
	return result, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("保存记录 %q 失败 %w", path, err)
	}
	return nil
}

func readJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("读取记录 %q 失败 %w", path, err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("解析记录 %q 失败 %w", path, err)
	}
	return value, nil
}
