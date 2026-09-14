package persistence

import "github.com/channing771/mornlea/packages/shared/core"

// SplitPhasedLoad 把已排好序的全量键切成首批与剩余：只切分不排序，调用方
// 负责按出生点距离排好 `all`。返回切片均为新分配，与 `all` 无共享。
func SplitPhasedLoad(all []core.ChunkKey, firstN int) (first, rest []core.ChunkKey) {
	if firstN <= 0 {
		rest = make([]core.ChunkKey, len(all))
		copy(rest, all)
		return nil, rest
	}
	if firstN >= len(all) {
		out := make([]core.ChunkKey, len(all))
		copy(out, all)
		return out, nil
	}
	first = make([]core.ChunkKey, firstN)
	copy(first, all[:firstN])
	rest = make([]core.ChunkKey, len(all)-firstN)
	copy(rest, all[firstN:])
	return first, rest
}
