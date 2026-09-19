package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/client/client"
)

func main() {
	baselinePath := flag.String("baseline", "", "基线 JSON")
	currentPath := flag.String("current", "", "当前 JSON")
	maxRegression := flag.Float64("max-regression", 0.20, "允许的最大相对退化")
	allowScenarioUpgrade := flag.String("allow-scenario-upgrade", "", "只允许显式的 22:23 场景迁移")
	godotReportPath := flag.String("godot-pilot-report", "", "校验 Godot 试点报告 JSON")
	godotVisualRun := flag.String("godot-visual-run", "", "Godot 试点抓帧目录")
	godotVisualGolden := flag.String("godot-visual-golden", "testdata/visual-golden", "同语义 tracked golden 根目录")
	godotVisualSemantics := flag.String("godot-visual-semantics", "testdata/godot-pilot/visual-semantics.json", "试点与 golden 的同语义映射")
	godotVisualOutput := flag.String("godot-visual-output", "", "比对分类报告输出路径")
	flag.Parse()

	if *godotReportPath != "" {
		if _, err := readGodotPilotReport(*godotReportPath); err != nil {
			fail("%v", err)
		}
		fmt.Println("Godot 试点报告身份完整")
		return
	}
	if *godotVisualRun != "" {
		table, err := readGodotPilotVisualSemantics(*godotVisualSemantics)
		if err != nil {
			fail("%v", err)
		}
		report, err := compareGodotPilotVisuals(*godotVisualRun, *godotVisualGolden, table)
		if err != nil {
			fail("%v", err)
		}
		output := *godotVisualOutput
		if output == "" {
			output = filepath.Join(*godotVisualRun, "compare-report.json")
		}
		if err := writeGodotPilotVisualCompareReport(output, report); err != nil {
			fail("%v", err)
		}
		fmt.Printf("Godot 视觉分类报告已写入 %s（%d 条）\n", output, len(report.Classifications))
		return
	}

	if *baselinePath == "" || *currentPath == "" {
		fail("-baseline 与 -current 都必须提供")
	}
	baseline := readReport(*baselinePath)
	current := readReport(*currentPath)
	records, err := compareReportsWithScenarioUpgrade(
		baseline, current, *maxRegression, *allowScenarioUpgrade,
	)
	if err != nil {
		fail("%v", err)
	}
	for _, record := range records {
		fmt.Fprintln(os.Stdout, "性能记录:", record)
	}
	fmt.Println(comparisonSuccessMessage(baseline.ScenarioVersion, current.ScenarioVersion))
}

func readReport(path string) client.PerfReport {
	data, err := os.ReadFile(path)
	if err != nil {
		fail("读取 %s: %v", path, err)
	}
	var report client.PerfReport
	if err := json.Unmarshal(data, &report); err != nil {
		fail("解析 %s: %v", path, err)
	}
	return report
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
