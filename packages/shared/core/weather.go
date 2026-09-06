package core

// 天气：服务端权威的晴/雨/雷暴三态。服务端每完成一个权威 tick 把剩余时长
// 恰好递减 1，到期按固定分布掷骰进入下一段；客户端只消费最新有效权威状态里
// 的值，不得用本地随机或墙钟自选。
//
// wire 值域固定为 0..2：0=晴、1=雨、2=雷暴。越界值由编解码层拒绝，
// 不进权威状态机。
type WeatherKind uint8

const (
	// WeatherClear 晴：默认天气，老存档迁移与新世界初始值。
	WeatherClear WeatherKind = iota
	// WeatherRain 雨：降水形态（雨/雪）由客户端按高度相对雪线本地派生，
	// 不进权威状态、不占同步字段。
	WeatherRain
	// WeatherThunder 雷暴：雨段内小概率升级，叠加有界全屏闪光，无伤害语义。
	WeatherThunder
)
