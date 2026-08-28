// Package protocol 定义端无关的消息协议层：封闭的 `ClientPacket`/
// `ServerPacket` packet 集合、冻结的包 ID 注册表与逐字段 Validate 规则。
// 编解码（wire 字节层）与会话编排分属 `internal/network` 的 codec 簇与
// 根包，本包不依赖二者。
package protocol

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// ClientMessage 是客户端可发送消息的封闭集合。
type ClientMessage interface {
	clientMessage()
}

// ServerMessage 是服务端可发送消息的封闭集合。
type ServerMessage interface {
	serverMessage()
}

func finite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func finiteVec3(value mgl32.Vec3) bool {
	for _, component := range value {
		if !finite32(component) {
			return false
		}
	}
	return true
}
