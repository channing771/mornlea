package archcheck_test

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// coreArmorPath 是护甲域值的唯一权威文件：槽位枚举、件→槽/点数/耐久真源表、
// `MaxArmorPoints`、四槽求和与减免公式全部定义在这一处。
const coreArmorPath = "packages/shared/core/armor.go"

// coreArmorDir 是允许声明护甲域类型与域入口符号的唯一目录；权威文件拆分重组
// 时在目录内进行即可，出目录即视为第二套域。
const coreArmorDir = "packages/shared/core"

// armorDomainSymbols 是护甲域的入口名：槽位类型、件→槽映射、四槽求和、减免
// 公式与点数上限。这些名字的**声明**只允许落在 core，别处同名声明即第二套
// 域实现。协议字段（`PlayerState.ArmorPoints`）、存档装备区与桥分节
// （`UIHudArmor`）是域值的合法投影，经符号引用消费——守卫只拦声明，不拦
// 引用，也不拦以域值为语义但不同名的投影字段。
var armorDomainSymbols = map[string]bool{
	"ArmorSlot":      true,
	"ArmorSlotOf":    true,
	"ArmorPoints":    true,
	"ReducedDamage":  true,
	"MaxArmorPoints": true,
}

// armorTableFingerprints 是护甲真源表的数值指纹：四件铁甲的耐久上限。单值在
// 生产代码里有大量无关命中（颜色字节、帧预算、坐标），不成其为证据；但一张
// 复制的表必然同时携带多个上限值，单一生产文件出现两个及以上不同指纹值即是
// 抄表。每件点数（2/6/5）与减免公式的常数（100/4）是过于常见的整数，无法
// 充当指纹；内联公式与改名单值复制是本守卫的已知盲区，由评审与域内向量测试
// 兜底——与难度守卫「表层侵蚀可拦、影子逻辑靠评审」的边界同一口径。
var armorTableFingerprints = map[string]bool{
	"165": true,
	"240": true,
	"225": true,
	"195": true,
}

// armorFingerprintThreshold 是单一文件触发抄表判定的不同指纹值个数下限：
// 取 2 是「无既有误报」与「任何成表复制必触发」之间的最小值（实测当前生产
// Go 源码没有任何域外文件同时含两个指纹值）。
const armorFingerprintThreshold = 2

// TestArmorStaysASingleCoreDomain 把护甲域的「单一真源」钉进源码门禁，守住
// 两条边界：
//   - 域入口符号（`ArmorSlot` 类型与 `ArmorSlotOf`/`ArmorPoints`/
//     `ReducedDamage`/`MaxArmorPoints`）只允许声明在 core，别处同名声明即
//     第二套枚举或第二套公式；
//   - 四件耐久上限的表值作为整数字面量只允许成组出现在权威文件，域外文件
//     同时出现多个指纹值即是在复制真源表。
//
// 守卫只覆盖生产 Go 源码（跳过 `_test.go`，沿难度守卫的豁免口径）：测试夹具
// 表达具体护甲件与点数是正当的消费行为。它防的是符号与表值层面的侵蚀；一套
// 不同名、不同字面的影子护甲逻辑仍需行为测试与评审发现，本守卫不替代。
func TestArmorStaysASingleCoreDomain(t *testing.T) {
	root := repositoryRoot(t)
	for _, module := range workspaceModules(t) {
		err := filepath.WalkDir(filepath.Join(root, module), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			findings, parseErr := armorDomainFindings(filepath.ToSlash(relative), source)
			if parseErr != nil {
				t.Fatalf("解析 %s: %v", path, parseErr)
			}
			for _, finding := range findings {
				t.Errorf("%s: %s", path, finding)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("扫描 %s 生产源码: %v", module, err)
		}
	}
}

// armorDomainFindings 是护甲守卫的纯函数内核：给一份生产 Go 源码与其仓库根
// 相对路径，返回失真清单（已含定位前缀）。注释不进入 token 流——文档注释里
// 讨论护甲域是正当的，不受本守卫约束。之所以做成「吃字节、不碰磁盘」的形状，
// 是为了让自检能喂内嵌坏样本走与全仓门禁完全相同的判定路径：先证明检测器会
// 红，再信任它报出的「0 条」。
func armorDomainFindings(relative string, source []byte) ([]string, error) {
	var findings []string
	fileSet := token.NewFileSet()
	var sourceScanner scanner.Scanner
	sourceFile := fileSet.AddFile(relative, -1, len(source))
	sourceScanner.Init(sourceFile, source, nil, 0)
	authoritative := relative == coreArmorPath
	fingerprints := make(map[string]bool)
	mentionsDomain := false
	for {
		_, kind, literal := sourceScanner.Scan()
		if kind == token.EOF {
			break
		}
		switch kind {
		case token.IDENT:
			if armorDomainSymbols[literal] {
				mentionsDomain = true
			}
		case token.INT:
			if !authoritative && armorTableFingerprints[literal] {
				fingerprints[literal] = true
			}
		}
	}
	if len(fingerprints) >= armorFingerprintThreshold {
		values := make([]string, 0, len(fingerprints))
		for value := range fingerprints {
			values = append(values, value)
		}
		sort.Strings(values)
		findings = append(findings, sourceFile.Position(sourceFile.Pos(0)).String()+
			": 域外抄表——文件同时出现耐久上限指纹 ["+strings.Join(values, " ")+
			"];护甲真源表只允许定义在 "+coreArmorPath+"，其他包只许经 core 的符号引用，不得复制表值")
	}
	if !mentionsDomain {
		return findings, nil
	}
	parsed, err := parser.ParseFile(fileSet, relative, source, 0)
	if err != nil {
		return nil, err
	}
	inCore := strings.HasPrefix(relative, coreArmorDir+"/")
	if inCore {
		return findings, nil
	}
	report := func(position token.Pos, kind, name string) {
		findings = append(findings, fileSet.Position(position).String()+
			": 声明了第二套护甲域"+kind+" "+name+";域入口只允许定义在 "+coreArmorDir)
	}
	for _, declaration := range parsed.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if armorDomainSymbols[declaration.Name.Name] {
				report(declaration.Name.Pos(), "入口", declaration.Name.Name)
			}
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				switch specification := specification.(type) {
				case *ast.TypeSpec:
					if armorDomainSymbols[specification.Name.Name] {
						report(specification.Name.Pos(), "类型", specification.Name.Name)
					}
				case *ast.ValueSpec:
					for _, name := range specification.Names {
						if armorDomainSymbols[name.Name] {
							report(name.Pos(), "常量", name.Name)
						}
					}
				}
			}
		}
	}
	return findings, nil
}

// TestArmorDomainGuardCatchesKnownBadSamples 用内嵌样本证伪护甲守卫：合法
// 引用与无关同值不得报，第二套域声明与域外抄表必须报。样本是**字符串常量**、
// 不是本文件的注释，不会被全仓标识符扫描当成提及。
func TestArmorDomainGuardCatchesKnownBadSamples(t *testing.T) {
	// 合法消费：只引用 `core.ReducedDamage`；单个无关的 240（帧预算）不构成
	// 抄表，也不含域符号声明。
	goodFindings, err := armorDomainFindings("packages/client/good.go", []byte("package sample\n"+
		"\n"+
		"import core \"example.com/mornlea/packages/shared/core\"\n"+
		"\n"+
		"// settle 用域公式计算有效伤害；maxFrames=240 是与护甲无关的帧预算。\n"+
		"func settle(damage int32, points uint8) int32 {\n"+
		"\tconst maxFrames = 240\n"+
		"\t_ = maxFrames\n"+
		"\treturn core.ReducedDamage(damage, points)\n"+
		"}\n"))
	if err != nil {
		t.Fatalf("解析合法样本: %v", err)
	}
	if len(goodFindings) != 0 {
		t.Errorf("合法引用被误报: %v", goodFindings)
	}

	// 坏样本一：core 之外声明同名域入口（第二套公式）。
	secondFindings, err := armorDomainFindings("packages/client/second.go", []byte("package sample\n"+
		"\n"+
		"// ReducedDamage 内联了一份减免公式，是第二套域实现。\n"+
		"func ReducedDamage(damage int32, points uint8) int32 { return damage }\n"))
	if err != nil {
		t.Fatalf("解析第二套域样本: %v", err)
	}
	if len(secondFindings) != 1 || !strings.Contains(secondFindings[0], "ReducedDamage") {
		t.Errorf("第二套域声明未被命中，想要恰好 1 条指向 ReducedDamage 的失真: %v", secondFindings)
	}

	// 坏样本二：域外文件同时携带两件耐久上限字面量（抄了半张表）。
	tableFindings, err := armorDomainFindings("packages/client/table.go", []byte("package sample\n"+
		"\n"+
		"// 抄了半张表：头盔与胸甲的耐久上限被原样复制。\n"+
		"var table = map[string]int{\"helmet\": 165, \"chestplate\": 240}\n"))
	if err != nil {
		t.Fatalf("解析抄表样本: %v", err)
	}
	if len(tableFindings) != 1 || !strings.Contains(tableFindings[0], "抄表") ||
		!strings.Contains(tableFindings[0], "165 240") {
		t.Errorf("域外抄表未被命中，想要恰好 1 条含指纹清单的失真: %v", tableFindings)
	}

	// 权威文件自身携带全部指纹值不报：豁免以文件为单位。
	authoritativeFindings, err := armorDomainFindings(coreArmorPath, []byte("package core\n"+
		"\n"+
		"const (\n"+
		"\thelmet = 165\n"+
		"\tchestplate = 240\n"+
		")\n"))
	if err != nil {
		t.Fatalf("解析权威文件样本: %v", err)
	}
	if len(authoritativeFindings) != 0 {
		t.Errorf("权威文件被误报: %v", authoritativeFindings)
	}
}
