// 本文件只保留 Agent Dialogue 与协议共同使用的有界常量和环境事实值。
// HTTP request/response 严格校验由 Agent v1 contract 承担，不再保留旧
// direct-model prompt、裸摘要请求或响应 envelope。
package companion

import (
	"fmt"

	"github.com/channing771/mornlea/internal/core"
)

const (
	// MaxDialogueLineBytes 是单句伙伴台词的 UTF-8 字节上限。
	MaxDialogueLineBytes = 256
	// MaxDialogueSummaryBytes 是 Agent memory summary 的 UTF-8 字节上限。
	MaxDialogueSummaryBytes = 2048
)

// DialogueEnvDigest 是一次台词请求可携带的极小附近环境摘要，与规划观察的
// 环境半部同构；切片按既有上界和稳定坐标顺序生成。
type DialogueEnvDigest struct {
	ExposedBlocks []PlanBlock
	Heights       []PlanHeight
}

// Validate 校验环境摘要的数量、方块、高度与稳定排序不变量。
func (d DialogueEnvDigest) Validate() error {
	if len(d.ExposedBlocks) > MaxPlanExposedBlocks {
		return fmt.Errorf("companion: dialogue 环境方块数 %d 超过上限 %d",
			len(d.ExposedBlocks), MaxPlanExposedBlocks)
	}
	for index, block := range d.ExposedBlocks {
		if block.Block == core.AirID || !core.RegisteredBlock(block.Block) {
			return fmt.Errorf("companion: dialogue 环境方块[%d] 编号 %d 非法（空气或未注册）",
				index, block.Block)
		}
		if !validPlanBlockY(block.Pos.Y) {
			return fmt.Errorf("companion: dialogue 环境方块[%d] Y=%d 越界", index, block.Pos.Y)
		}
		if index > 0 && !planBlockAfter(block.Pos, d.ExposedBlocks[index-1].Pos) {
			return fmt.Errorf("companion: dialogue 环境方块[%d] 未按 (X,Y,Z) 严格升序", index)
		}
	}
	if len(d.Heights) > MaxPlanHeightSamples {
		return fmt.Errorf("companion: dialogue 高度样本数 %d 超过上限 %d",
			len(d.Heights), MaxPlanHeightSamples)
	}
	for index, height := range d.Heights {
		if height.Height != core.MinY-1 && !validPlanBlockY(height.Height) {
			return fmt.Errorf("companion: dialogue 高度样本[%d] Height=%d 越界", index, height.Height)
		}
		if index > 0 {
			previous := d.Heights[index-1]
			if previous.X > height.X || previous.X == height.X && previous.Z >= height.Z {
				return fmt.Errorf("companion: dialogue 高度样本[%d] 未按 (X,Z) 严格升序", index)
			}
		}
	}
	return nil
}
