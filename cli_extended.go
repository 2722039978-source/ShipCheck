//go:build !shipcheck_gui

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func reviseCLI(args []string) int {
	flags := commandFlags("revise", "shipcheck revise --evidence DIR [--staging DIR] [--results DIR] [-- TOOL ARG ...]")
	evidence := flags.String("evidence", "", "原失败记录目录")
	staging := flags.String("staging", "", "修正后的输入目录；省略则使用原输入")
	results := flags.String("results", ".shipcheck", "保存修正尝试的目录")
	if err := flags.Parse(args); err != nil {
		return flagExit(err)
	}
	if *evidence == "" || *results == "" || (*staging == "" && flags.NArg() == 0) {
		flags.Usage()
		return exitError
	}
	var command *commandSpec
	if flags.NArg() > 0 {
		command = &commandSpec{Tool: flags.Arg(0), Args: append([]string{}, flags.Args()[1:]...)}
	}
	result, original, err := revisePackaging(*evidence, *staging, *results, command)
	if err != nil {
		return printError(err)
	}
	result.writeTo(os.Stdout)
	writeMissingComparison(os.Stdout, original, result.Verification)
	fmt.Println("修正结果目录", result.Directory)
	if result.passed() {
		return exitPassed
	}
	return exitRejected
}

func historyCLI(args []string) int {
	flags := commandFlags("history", "shipcheck history [--results DIR] [--order-id ID] [--version VERSION] [--status passed|rejected] [--from-date YYYY-MM-DD] [--to-date YYYY-MM-DD] [--json]")
	results := flags.String("results", ".shipcheck", "历史记录目录")
	orderID := flags.String("order-id", "", "工单号")
	version := flags.String("version", "", "起始或目标版本")
	status := flags.String("status", "", "passed 或 rejected")
	fromDate := flags.String("from-date", "", "起始日期，包含当天")
	toDate := flags.String("to-date", "", "结束日期，包含当天")
	asJSON := flags.Bool("json", false, "输出 JSON")
	if err := flags.Parse(args); err != nil {
		return flagExit(err)
	}
	if *results == "" || flags.NArg() != 0 {
		flags.Usage()
		return exitError
	}
	entries, err := listHistory(*results, historyFilter{OrderID: *orderID, Version: *version, Status: *status, FromDate: *fromDate, ToDate: *toDate})
	if err != nil {
		return printError(err)
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(entries); err != nil {
			return printError(err)
		}
		return exitPassed
	}
	for _, entry := range entries {
		fmt.Printf("%s  %s  %s → %s  %s  %s\n  %s\n", entry.CreatedAt, entry.OrderID, entry.FromVersion, entry.ToVersion, entry.Kind, entry.Status, entry.Directory)
	}
	fmt.Printf("共 %d 条记录\n", len(entries))
	return exitPassed
}

func orderCLI(args []string) int {
	if len(args) == 0 {
		return printError(fmt.Errorf("用法 shipcheck order <validate|create> [选项]"))
	}
	switch args[0] {
	case "validate":
		flags := commandFlags("order validate", "shipcheck order validate --order ORDER.json")
		path := flags.String("order", "", "工单 JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return flagExit(err)
		}
		if *path == "" || flags.NArg() != 0 {
			flags.Usage()
			return exitError
		}
		ticket, _, err := loadOrder(*path)
		if err != nil {
			return printError(err)
		}
		fmt.Println("工单有效", ticket.OrderID)
		return exitPassed
	case "create":
		flags := commandFlags("order create", "shipcheck order create --template TEMPLATE.json --id ID [--from VERSION] [--to VERSION] --output ORDER.json")
		path := flags.String("template", "", "模板 JSON")
		id := flags.String("id", "", "新工单号")
		from := flags.String("from", "", "起始版本；省略则使用模板值")
		to := flags.String("to", "", "目标版本；省略则使用模板值")
		output := flags.String("output", "", "新工单保存路径")
		if err := flags.Parse(args[1:]); err != nil {
			return flagExit(err)
		}
		if *path == "" || *id == "" || *output == "" || flags.NArg() != 0 {
			flags.Usage()
			return exitError
		}
		value, err := loadTemplate(*path)
		if err != nil {
			return printError(err)
		}
		ticket, err := instantiateTemplate(value, *id, *from, *to)
		if err != nil {
			return printError(err)
		}
		if err := saveOrder(*output, ticket); err != nil {
			return printError(err)
		}
		fmt.Println("工单已保存", *output)
		return exitPassed
	default:
		return printError(fmt.Errorf("未知工单操作 %q", args[0]))
	}
}

func templateCLI(args []string) int {
	if len(args) == 0 || args[0] != "save" {
		return printError(fmt.Errorf("用法 shipcheck template save --name NAME --version VERSION --order ORDER.json --output TEMPLATE.json [--zip-output PACKAGE.zip] [-- TOOL ARG ...]"))
	}
	flags := commandFlags("template save", "shipcheck template save --name NAME --version VERSION --order ORDER.json --output TEMPLATE.json [--zip-output PACKAGE.zip] [-- TOOL ARG ...]")
	name := flags.String("name", "", "模板名称")
	version := flags.String("version", "", "模板版本")
	orderPath := flags.String("order", "", "规则来源工单")
	output := flags.String("output", "", "模板保存路径")
	zipOutput := flags.String("zip-output", "upgrade.zip", "打包 ZIP 相对路径")
	if err := flags.Parse(args[1:]); err != nil {
		return flagExit(err)
	}
	if *name == "" || *version == "" || *orderPath == "" || *output == "" {
		flags.Usage()
		return exitError
	}
	ticket, _, err := loadOrder(*orderPath)
	if err != nil {
		return printError(err)
	}
	value := orderTemplate{Name: *name, Version: *version, Order: ticket, Output: *zipOutput}
	if flags.NArg() > 0 {
		value.Command = commandSpec{Tool: flags.Arg(0), Args: append([]string{}, flags.Args()[1:]...)}
	}
	if err := saveTemplate(*output, value); err != nil {
		return printError(err)
	}
	fmt.Println("模板已保存", *output)
	return exitPassed
}
