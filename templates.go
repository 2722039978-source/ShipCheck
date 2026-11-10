package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type orderTemplate struct {
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Order   order       `json:"order"`
	Output  string      `json:"output"`
	Command commandSpec `json:"command"`
}

type templateReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

func decodeTemplateJSON[T any](data []byte) (T, error) {
	var empty T
	var value *T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return empty, err
	}
	if value == nil {
		return empty, errors.New("内容必须为 JSON 对象")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return empty, err
		}
		return empty, errors.New("JSON 对象之后包含额外内容")
	}
	return *value, nil
}

func parseOrderJSON(data []byte) (order, error) {
	ticket, err := decodeTemplateJSON[order](data)
	if err != nil {
		return order{}, fmt.Errorf("解析工单失败 %w", err)
	}
	if err := validateOrder(ticket); err != nil {
		return order{}, err
	}
	return ticket, nil
}

func loadTemplate(path string) (orderTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return orderTemplate{}, fmt.Errorf("读取模板 %q 失败 %w", path, err)
	}
	value, err := decodeTemplateJSON[orderTemplate](data)
	if err != nil {
		return orderTemplate{}, fmt.Errorf("解析模板 %q 失败 %w", path, err)
	}
	if err := validateTemplate(value); err != nil {
		return orderTemplate{}, fmt.Errorf("模板 %q 无效 %w", path, err)
	}
	return value, nil
}

func validateTemplate(value orderTemplate) error {
	if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Version) == "" {
		return errors.New("模板名称和版本必须为非空字符串")
	}
	if err := validateOrder(value.Order); err != nil {
		return fmt.Errorf("模板工单规则无效 %w", err)
	}
	if _, err := localOutputPath(value.Output); err != nil {
		return fmt.Errorf("模板默认输出无效 %w", err)
	}
	inputs := requiredPaths(value.Order)
	for _, name := range inputs {
		if strings.EqualFold(name, value.Output) {
			return fmt.Errorf("模板默认输出 %q 与要求的输入文件冲突", value.Output)
		}
	}
	if err := validateCombinedPaths(append(inputs, value.Output)); err != nil {
		return fmt.Errorf("模板默认输出与输入路径冲突 %w", err)
	}
	command := value.Command
	if command.Tool == "" && command.Executable == "" && command.WorkspaceTool == "" && len(command.Args) == 0 {
		return nil
	}
	if strings.TrimSpace(command.Tool) == "" || strings.ContainsRune(command.Tool, '\x00') {
		return errors.New("模板默认命令的 tool 必须为有效非空字符串")
	}
	if !filepath.IsAbs(command.Tool) && strings.ContainsAny(command.Tool, "/\\") && !filepath.IsLocal(command.Tool) {
		return errors.New("模板默认命令的 tool 相对路径必须位于输入目录内")
	}
	if command.Executable != "" && (!filepath.IsAbs(command.Executable) || strings.ContainsRune(command.Executable, '\x00')) {
		return errors.New("模板默认命令的 executable 必须为绝对路径")
	}
	if command.WorkspaceTool != "" {
		if !validRelativePath(command.WorkspaceTool) {
			return errors.New("模板默认命令的 workspace_tool 必须为有效包内相对路径")
		}
		if _, err := filepath.Localize(command.WorkspaceTool); err != nil {
			return fmt.Errorf("模板默认命令的 workspace_tool 无法用于当前系统 %w", err)
		}
	}
	for _, argument := range command.Args {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("模板默认命令的参数不能包含 NUL 字符")
		}
	}
	return nil
}

func saveTemplate(path string, value orderTemplate) error {
	if err := validateTemplate(value); err != nil {
		return err
	}
	return saveNewJSON(path, value)
}

func instantiateTemplate(value orderTemplate, id, from, to string) (order, error) {
	if err := validateTemplate(value); err != nil {
		return order{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return order{}, err
	}
	var copied orderTemplate
	if err := json.Unmarshal(data, &copied); err != nil {
		return order{}, err
	}
	ticket := copied.Order
	ticket.OrderID = id
	if from != "" {
		ticket.FromVersion = from
	}
	if to != "" {
		ticket.ToVersion = to
	}
	digest := sha256.Sum256(data)
	ticket.Template = &templateReference{Name: value.Name, Version: value.Version, SHA256: hex.EncodeToString(digest[:])}
	if err := validateOrder(ticket); err != nil {
		return order{}, err
	}
	return ticket, nil
}

func saveOrder(path string, value order) error {
	if err := validateOrder(value); err != nil {
		return err
	}
	return saveNewJSON(path, value)
}

func saveNewJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := copyFile(path, bytes.NewReader(append(data, '\n'))); err != nil {
		return fmt.Errorf("保存文件 %q 失败 %w", path, err)
	}
	return nil
}
