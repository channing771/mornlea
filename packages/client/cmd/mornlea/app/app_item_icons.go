//go:build darwin

package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

const itemIconDataURIPrefix = "data:image/png;base64,"

// itemIconCatalog 是应用装配期生成的只读物品呈现目录。每个注册物品的 16×16
// RGBA 只编码一次；后续 HUD、面板与配方组装只做数组索引和字符串复用。
type itemIconCatalog struct {
	items [core.ItemIDMax]client.UIItemMetadata
}

func newItemIconCatalog(registry *assets.Registry) (itemIconCatalog, error) {
	return buildItemIconCatalog(registry, encodeItemIconDataURI)
}

func buildItemIconCatalog(registry *assets.Registry, encode func([]byte) (string, error)) (itemIconCatalog, error) {
	if registry == nil || encode == nil {
		return itemIconCatalog{}, errors.New("物品图标目录缺少注册表或编码器")
	}
	var catalog itemIconCatalog
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if !core.RegisteredItem(item) {
			continue
		}
		pixels, ok := registry.ItemIconRGBA(item)
		if !ok {
			return itemIconCatalog{}, fmt.Errorf("注册物品 %d 缺少图标", item)
		}
		icon, err := encode(pixels)
		if err != nil {
			return itemIconCatalog{}, fmt.Errorf("编码物品 %d 图标: %w", item, err)
		}
		if !strings.HasPrefix(icon, itemIconDataURIPrefix) || len(icon) > client.UIIconMaxChars {
			return itemIconCatalog{}, fmt.Errorf("物品 %d 图标 data URI 越界", item)
		}
		name, ok := core.ItemDisplayName(item)
		if !ok {
			return itemIconCatalog{}, fmt.Errorf("注册物品 %d 缺少显示名", item)
		}
		catalog.items[item] = client.UIItemMetadata{Name: name, Icon: icon}
	}
	return catalog, nil
}

// UIItemMetadata 实现 client.UIItemMetadataSource。目录按 `ItemIDMax` 定长，
// 未知、空物品与未注册物品统一返回 false。
func (c *itemIconCatalog) UIItemMetadata(item core.ItemID) (client.UIItemMetadata, bool) {
	if c == nil || item == core.ItemNone || item >= core.ItemIDMax || !core.RegisteredItem(item) {
		return client.UIItemMetadata{}, false
	}
	metadata := c.items[item]
	return metadata, metadata.Name != "" && metadata.Icon != ""
}

func encodeItemIconDataURI(pixels []byte) (string, error) {
	if len(pixels) != 16*16*4 {
		return "", fmt.Errorf("RGBA 字节数=%d，想要 %d", len(pixels), 16*16*4)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, &image.RGBA{
		Pix:    pixels,
		Stride: 16 * 4,
		Rect:   image.Rect(0, 0, 16, 16),
	}); err != nil {
		return "", err
	}
	return itemIconDataURIPrefix + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
