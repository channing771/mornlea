package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件是写入方入队入口的唯一性守卫：权威方块写入后的定时面反activate必须
// 经 `recordChange`（内部转交 realm 的统一门面 EnqueueBlockWrite）这一个入口
// 接线，写入方不得按自己的语义挑入队面（流体域、湿窗口、单格湿度候选）——
// 策略只有一个真源，新增写入方时也只有这一个入口要接线。细粒度入队方法
// （realm.State 的 EnqueueFluidUpdate/EnqueueFarmlandMoisture/
// EnqueueFarmlandMoistureAroundFluid）只保留给测试夹具直连。

// enqueueEntryGuardFiles 返回守卫覆盖的生产源文件名集合（不含 _test.go）。
// engine_changes.go 是唯一豁免文件——recordChange 本体就在那里。
// entityEnqueueEntryViolations 检查生产源文件集合：engine_changes.go 是唯一
// 入口（必须经统一门面入队、不得回落细粒度方法），其余生产文件不得出现任何
// 入队/登记面的直接引用。纯字符串匹配已足够——目标是钉住「写入方只有一个
// 入口要接线」的组织约束，不是类型级分析。
func entityEnqueueEntryViolations(sources map[string]string) []string {
	forbidden := []string{
		"EnqueueFluidUpdate",
		"EnqueueFarmlandMoisture",
		"EnqueueBlockWrite",
		"FluidQueue(",
		".Record(",
	}
	violations := make([]string, 0)
	for name, source := range sources {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == "engine_changes.go" {
			// 唯一入口本体：必须经统一门面入队，且不得回落到细粒度方法
			//（门面自身的名字允许出现在调用与注释里）。
			if !strings.Contains(source, "engine.realm.EnqueueBlockWrite(") {
				violations = append(violations, "engine_changes.go 的 recordChange 未经 realm 统一门面 EnqueueBlockWrite 入队")
			}
			for _, keyword := range forbidden[:2] {
				if strings.Contains(source, keyword) {
					violations = append(violations, "engine_changes.go 直接引用了细粒度入队方法 "+keyword)
				}
			}
			continue
		}
		for _, keyword := range forbidden {
			if strings.Contains(source, keyword) {
				violations = append(violations, name+" 绕过统一入队入口直接引用 "+keyword)
			}
		}
	}
	return violations
}

func TestEntityBlockWriteEnqueueHasSingleEntry(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取 entity 包目录：%v", err)
	}
	sources := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(".", entry.Name()))
		if err != nil {
			t.Fatalf("读取 %s：%v", entry.Name(), err)
		}
		sources[entry.Name()] = string(source)
	}
	if _, ok := sources["engine_changes.go"]; !ok {
		t.Fatal("未找到唯一入队入口文件 engine_changes.go")
	}
	if violations := entityEnqueueEntryViolations(sources); len(violations) != 0 {
		t.Fatalf("写入方入队入口漂移：%s", strings.Join(violations, "；"))
	}
}

// TestEnqueueEntryGuardRejectsSyntheticBypass 证明守卫真的咬人：写入方自带
// 细粒度入队调用、或 recordChange 丢失门面调用时都必须被判违规，避免守卫
// 退化成永绿。
func TestEnqueueEntryGuardRejectsSyntheticBypass(t *testing.T) {
	clean := map[string]string{
		"engine_changes.go": `package entity

func (engine *engineContext) recordChange(position core.BlockPos) {
	pending.Record(0, position, 0)
	engine.realm.EnqueueBlockWrite(0, position, 0, 0)
}
`,
		"writer.go": `package entity

func (engine *engineContext) plainWrite(position core.BlockPos) {
	engine.recordChange(position)
}
`,
	}
	if violations := entityEnqueueEntryViolations(clean); len(violations) != 0 {
		t.Fatalf("合法单入口形状被误拒绝：%s", strings.Join(violations, "；"))
	}

	bypass := map[string]string{
		"engine_changes.go": clean["engine_changes.go"],
		"writer.go": `package entity

func (engine *engineContext) wetWrite(position core.BlockPos) {
	engine.realm.EnqueueFarmlandMoistureAroundFluid(0, position)
	engine.recordChange(position)
}
`,
	}
	if violations := entityEnqueueEntryViolations(bypass); len(violations) == 0 {
		t.Fatal("写入方直连细粒度入队未被守卫识别")
	}

	noFacade := map[string]string{
		"engine_changes.go": `package entity

func (engine *engineContext) recordChange(position core.BlockPos) {
	pending.Record(0, position, 0)
	engine.realm.EnqueueFluidUpdate(0, position)
}
`,
	}
	if violations := entityEnqueueEntryViolations(noFacade); len(violations) == 0 {
		t.Fatal("recordChange 回落到细粒度入队未被守卫识别")
	}
}
