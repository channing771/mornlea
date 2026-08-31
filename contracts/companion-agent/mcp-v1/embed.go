// Package mcpv1 嵌入已检入的 MCP v1 machine contract。调用方只取得独立
// 字节副本，并在组件构造时严格解析、失败关闭。
package mcpv1

import _ "embed"

var (
	//go:embed manifest.json
	manifestJSON []byte
	//go:embed schema.json
	schemaJSON []byte
)

// ManifestJSON 返回 manifest.json 的独立副本。
func ManifestJSON() []byte { return append([]byte(nil), manifestJSON...) }

// SchemaJSON 返回 schema.json 的独立副本。
func SchemaJSON() []byte { return append([]byte(nil), schemaJSON...) }
