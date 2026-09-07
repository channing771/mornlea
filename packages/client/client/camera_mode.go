package client

// CameraMode 是本地三态视角：0=第一人称、1=第三人称背面、2=第三人称正面。
// 纯本地呈现状态，不进入任何服务端消息、协议或存档字段；跨世界保留由调用方
// 经本地设置落盘达成。本任务只定状态与循环顺序，模型显隐与相机后拉由后续
// 视角任务消费同一类型。
type CameraMode int

const (
	// CameraFirstPerson 是默认视角：眼睛直连渲染，不渲染自身身体。
	CameraFirstPerson CameraMode = 0
	// CameraThirdPersonBack 是第三人称背面：相机位于眼睛沿视线反方向。
	CameraThirdPersonBack CameraMode = 1
	// CameraThirdPersonFront 是第三人称正面：相机位于眼睛沿视线正方向。
	CameraThirdPersonFront CameraMode = 2
)

// Valid 报告模式是否落在三态合法域内；配置文件与启动参数的越界值由调用方
// 落回第一人称。
func (mode CameraMode) Valid() bool {
	return mode >= CameraFirstPerson && mode <= CameraThirdPersonFront
}

// Next 按第一人称→背面→正面→第一人称的固定顺序推进，F5 循环的唯一状态机。
func (mode CameraMode) Next() CameraMode {
	return (mode + 1) % 3
}
