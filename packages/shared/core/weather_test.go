package core

import "testing"

// 天气种类的 wire 值域钉死为 0..2（晴/雨/雷暴）：协议与存档都直接依赖这组
// 取值，增减或重排必须同步升级协议版本，不得静默发生。
func TestWeatherKindWireValuesAreFrozen(t *testing.T) {
	if WeatherClear != 0 || WeatherRain != 1 || WeatherThunder != 2 {
		t.Fatalf("天气取值 = (%d, %d, %d)，想要 (0, 1, 2)",
			WeatherClear, WeatherRain, WeatherThunder)
	}
}
