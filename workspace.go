package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var errOutputMissing = errors.New("没有生成预期的 ZIP")

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q 必须为目录", path)
	}
	return resolved, nil
}

func resolveDestination(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(path)
	if !errors.Is(err, fs.ErrNotExist) || parent == path {
		return "", err
	}
	resolvedParent, err := resolveDestination(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func newResultDirectory(path, protectedDir, prefix string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	results, err := resolveDestination(absolute)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(protectedDir, results)
	if err == nil && filepath.IsLocal(relative) {
		return "", fmt.Errorf("结果目录 %q 必须位于 %q 之外", results, protectedDir)
	}
	if err := os.MkdirAll(results, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(results, prefix)
}

func localOutputPath(name string) (string, error) {
	if !validRelativePath(name) {
		return "", fmt.Errorf("输出路径 %q 必须为使用斜杠分隔的相对路径", name)
	}
	local, err := filepath.Localize(name)
	if err != nil {
		return "", fmt.Errorf("输出路径 %q 无法用于当前系统 %w", name, err)
	}
	return local, nil
}

func copyInputs(sourceDir, destinationDir string) (map[string]struct{}, error) {
	source, err := os.OpenRoot(sourceDir)
	if err != nil {
		return nil, err
	}
	defer source.Close()

	files := make(map[string]struct{})
	err = fs.WalkDir(source.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return err
		}
		target := filepath.Join(destinationDir, local)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("暂存输入 %q 必须为普通文件或目录", name)
		}
		if err := copyInputFile(source, local, target); err != nil {
			return fmt.Errorf("复制暂存输入 %q 失败 %w", name, err)
		}
		files[name] = struct{}{}
		return nil
	})
	return files, err
}

func copyInputFile(source *os.Root, name, target string) error {
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q 必须为普通文件", name)
	}
	if err := copyFile(target, input); err != nil {
		return err
	}
	if err := os.Chmod(target, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(target, time.Time{}, info.ModTime())
}

func copyFile(target string, input io.Reader) error {
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return errors.Join(err, os.Remove(target))
	}
	return nil
}

func captureZIP(workspace *os.Root, output, resultDir string) (string, error) {
	info, err := workspace.Lstat(output)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errOutputMissing
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("输出路径 %q 必须为普通文件", output)
	}
	file, err := workspace.Open(output)
	if err != nil {
		return "", err
	}
	defer file.Close()

	artifactDir := filepath.Join(resultDir, "artifact")
	if err := os.Mkdir(artifactDir, 0700); err != nil {
		return "", err
	}
	savedPath := filepath.Join(artifactDir, filepath.Base(output))
	if err := copyFile(savedPath, file); err != nil {
		return "", err
	}
	return savedPath, nil
}
