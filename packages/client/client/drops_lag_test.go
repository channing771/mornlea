package client_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestItemDropsRecordUpsertTick 锁定掉落关联的时序事实：每次 upsert 记录批次
// 的权威 tick（死亡关联窗口的唯一时钟），重复 upsert 刷新为最新 tick。
func TestItemDropsRecordUpsertTick(t *testing.T) {
	mirror := client.NewItemDrops()
	id := dropID(0, 1, 1)
	if err := mirror.Apply(network.ItemDropUpserts{
		ServerTick: 42,
		Drops:      []network.ItemDrop{dropUpsert(id, 1)},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := mirror.Presentations(); len(got) != 1 || got[0].UpsertTick != 42 {
		t.Fatalf("掉落呈现=%+v，想要 UpsertTick=42", got)
	}
	if err := mirror.Apply(network.ItemDropUpserts{
		ServerTick: 50,
		Drops:      []network.ItemDrop{dropUpsert(id, 1)},
	}); err != nil {
		t.Fatalf("重复 upsert: %v", err)
	}
	if got := mirror.Presentations(); len(got) != 1 || got[0].UpsertTick != 50 {
		t.Fatalf("重复 upsert 后呈现=%+v，想要 UpsertTick=50", got)
	}
}
