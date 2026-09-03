package companion

import (
	"bytes"
	"encoding/json"
)

// isJSONNull 区分显式 null 与字段缺席，供严格 JSON union 解码复用。
func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}
