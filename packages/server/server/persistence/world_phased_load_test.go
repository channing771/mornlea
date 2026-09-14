package persistence

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestSplitPhasedLoadKeepsOrder(t *testing.T) {
	t.Parallel()
	var a, b, c core.ChunkKey
	first, rest := SplitPhasedLoad([]core.ChunkKey{a, b, c}, 2)
	if len(first) != 2 || len(rest) != 1 {
		t.Fatalf("got %d/%d, want 2/1", len(first), len(rest))
	}
	if first[0] != a || first[1] != b || rest[0] != c {
		t.Fatalf("order changed")
	}
}

// TestSplitPhasedLoadBoundaries 覆盖首批边界：零/负数、等于/超出全量、空输入。
func TestSplitPhasedLoadBoundaries(t *testing.T) {
	t.Parallel()
	newKeys := func(n int) []core.ChunkKey {
		out := make([]core.ChunkKey, n)
		for i := range out {
			out[i] = core.ChunkKey{Dimension: core.DimensionID(i + 1)}
		}
		return out
	}
	cases := []struct {
		name      string
		n         int
		firstN    int
		wantFirst int
		wantRest  int
	}{
		{"零首批", 3, 0, 0, 3},
		{"负首批", 3, -2, 0, 3},
		{"首批等于全量", 3, 3, 3, 0},
		{"首批超出全量", 3, 9, 3, 0},
		{"空输入", 0, 2, 0, 0},
		{"空输入零首批", 0, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			all := newKeys(tc.n)
			first, rest := SplitPhasedLoad(all, tc.firstN)
			if len(first) != tc.wantFirst || len(rest) != tc.wantRest {
				t.Fatalf("got %d/%d, want %d/%d", len(first), len(rest), tc.wantFirst, tc.wantRest)
			}
			for i, k := range first {
				if k != all[i] {
					t.Fatalf("首批顺序变化")
				}
			}
			for i, k := range rest {
				if k != all[len(first)+i] {
					t.Fatalf("剩余顺序变化")
				}
			}
		})
	}
}

// TestSplitPhasedLoadNilInput 空输入不断言 nil，只断言长度为零。
func TestSplitPhasedLoadNilInput(t *testing.T) {
	t.Parallel()
	for _, firstN := range []int{-1, 0, 2} {
		first, rest := SplitPhasedLoad(nil, firstN)
		if len(first) != 0 || len(rest) != 0 {
			t.Fatalf("firstN=%d: got %d/%d, want 0/0", firstN, len(first), len(rest))
		}
	}
}

// TestSplitPhasedLoadNoSharedWrite 写返回切片不得污染入参。
func TestSplitPhasedLoadNoSharedWrite(t *testing.T) {
	t.Parallel()
	mk := func() []core.ChunkKey {
		return []core.ChunkKey{{Dimension: 1}, {Dimension: 2}, {Dimension: 3}}
	}
	all := mk()
	first, rest := SplitPhasedLoad(all, 2)
	first[0].Dimension = 100
	rest[0].Dimension = 200
	if !slices.Equal(all, mk()) {
		t.Fatalf("中间切分写返回切片污染了入参")
	}
	full, _ := SplitPhasedLoad(all, len(all))
	full[1].Dimension = 300
	if !slices.Equal(all, mk()) {
		t.Fatalf("全量首批写返回切片污染了入参")
	}
	_, restOnly := SplitPhasedLoad(all, 0)
	restOnly[0].Dimension = 400
	if !slices.Equal(all, mk()) {
		t.Fatalf("空首批写返回切片污染了入参")
	}
}
