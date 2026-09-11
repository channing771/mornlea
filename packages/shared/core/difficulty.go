package core

import "fmt"

// Difficulty 是服务端权威的世界难度三档域值。难度在创建世界时固定，由世界
// metadata 持久化并在权威模拟构造时注入，生命周期内不可变；客户端不持有、
// 不预测该值，难度差异只经服务端行为间接可观察。
//
// wire 值域固定为 0..2：0=normal、1=peaceful、2=hard。normal 是零值：旧档
// 迁移与未显式指定难度的新世界都落在 normal。越界值由存储编解码与模拟构造
// 边界分别拒绝，不进权威状态。
type Difficulty uint8

const (
	// DifficultyNormal 普通难度：既有生存规则的基准档，也是唯一的零值档。
	DifficultyNormal Difficulty = iota
	// DifficultyPeaceful 和平难度：跳过饥饿伤害、取消自然回血的饥饿门控，
	// 并在入口禁用夜行者生成。
	DifficultyPeaceful
	// DifficultyHard 困难难度：取消饥饿伤害的一点生命硬地板，饥饿可致死。
	DifficultyHard
)

// Valid 报告难度能否进入 metadata 编解码与权威模拟构造。
func (difficulty Difficulty) Valid() bool {
	return difficulty == DifficultyNormal ||
		difficulty == DifficultyPeaceful ||
		difficulty == DifficultyHard
}

// String 返回难度的小写规范文本，与 `ParseDifficulty` 互逆；非法值返回携带
// 数值的占位文本，仅供诊断输出，不得再被解析回权威状态。
func (difficulty Difficulty) String() string {
	switch difficulty {
	case DifficultyNormal:
		return "normal"
	case DifficultyPeaceful:
		return "peaceful"
	case DifficultyHard:
		return "hard"
	default:
		return fmt.Sprintf("difficulty(%d)", uint8(difficulty))
	}
}

// ParseDifficulty 严格解析小写难度文本，是 CLI 参数等文本输入的唯一入口；
// 大写、空白或未知文本稳定失败，不承担大小写宽容。
func ParseDifficulty(text string) (Difficulty, error) {
	switch text {
	case "normal":
		return DifficultyNormal, nil
	case "peaceful":
		return DifficultyPeaceful, nil
	case "hard":
		return DifficultyHard, nil
	default:
		return DifficultyNormal, fmt.Errorf("core: unknown difficulty %q", text)
	}
}
