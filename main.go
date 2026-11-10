//go:build !shipcheck_gui

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

const (
	exitPassed   = 0
	exitRejected = 1
	exitError    = 2
)

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

func configurePackagingProcess(*exec.Cmd) {}

func dispatch(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法 shipcheck <verify|run|replay|revise|history|order|template> [选项]")
		return exitError
	}

	switch args[0] {
	case "verify":
		return verifyCLI(args[1:])
	case "run":
		return runCLI(args[1:])
	case "replay":
		return replayCLI(args[1:])
	case "revise":
		return reviseCLI(args[1:])
	case "history":
		return historyCLI(args[1:])
	case "order":
		return orderCLI(args[1:])
	case "template":
		return templateCLI(args[1:])
	case "-h", "--help", "help":
		fmt.Println("用法 shipcheck <verify|run|replay|revise|history|order|template> [选项]")
		return exitPassed
	default:
		return printError(fmt.Errorf("未知命令 %q", args[0]))
	}
}

func verifyCLI(args []string) int {
	flags := commandFlags("verify", "shipcheck verify --order ORDER.json --zip PACKAGE.zip")
	orderPath := flags.String("order", "", "工单 JSON 路径")
	zipPath := flags.String("zip", "", "交付 ZIP 路径")
	results := flags.String("results", ".shipcheck", "保存验收报告和 ZIP 的目录")
	if err := flags.Parse(args); err != nil {
		return flagExit(err)
	}
	if *orderPath == "" || *zipPath == "" || *results == "" || flags.NArg() != 0 {
		flags.Usage()
		return exitError
	}

	saved, err := verifyAndRecord(*orderPath, *zipPath, *results)
	if err != nil {
		return printError(err)
	}

	saved.Report.writeTo(os.Stdout)
	fmt.Println("验收结果目录", saved.Directory)
	if saved.Report.Status == statusPassed {
		return exitPassed
	}
	return exitRejected
}

func runCLI(args []string) int {
	flags := commandFlags("run", "shipcheck run --order ORDER.json --staging DIR --output RELATIVE.zip -- TOOL [ARG ...]")
	orderPath := flags.String("order", "", "工单 JSON 路径")
	staging := flags.String("staging", "", "打包前的暂存目录")
	output := flags.String("output", "", "工作目录内预期生成的 ZIP 相对路径")
	results := flags.String("results", ".shipcheck", "保存交付包和证据的目录")
	if err := flags.Parse(args); err != nil {
		return flagExit(err)
	}
	if *orderPath == "" || *staging == "" || *output == "" || *results == "" || flags.NArg() == 0 || flags.Arg(0) == "" {
		flags.Usage()
		return exitError
	}

	command := commandSpec{Tool: flags.Arg(0), Args: append([]string{}, flags.Args()[1:]...)}
	result, err := runPackaging(*orderPath, *staging, *output, *results, command)
	if err != nil {
		return printError(err)
	}
	result.writeTo(os.Stdout)
	if result.passed() {
		return exitPassed
	}
	fmt.Println("证据目录", result.Directory)
	return exitRejected
}

func replayCLI(args []string) int {
	flags := commandFlags("replay", "shipcheck replay --evidence DIR")
	evidence := flags.String("evidence", "", "失败的 run 证据目录")
	results := flags.String("results", ".shipcheck", "保存复跑结果的目录")
	if err := flags.Parse(args); err != nil {
		return flagExit(err)
	}
	if *evidence == "" || *results == "" || flags.NArg() != 0 {
		flags.Usage()
		return exitError
	}

	result, original, err := replayPackaging(*evidence, *results)
	if err != nil {
		return printError(err)
	}
	result.writeTo(os.Stdout)
	writeMissingComparison(os.Stdout, original, result.Verification)
	fmt.Println("复跑结果目录", result.Directory)
	if result.passed() {
		return exitPassed
	}
	return exitRejected
}

func commandFlags(name, usage string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "用法", usage)
		flags.PrintDefaults()
	}
	return flags
}

func flagExit(err error) int {
	if err == flag.ErrHelp {
		return exitPassed
	}
	return exitError
}

func printError(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return exitError
}
