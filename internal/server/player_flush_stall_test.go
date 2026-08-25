package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPlayerFlushStallReturnsBoundedErrorAndKeepsDirty 捕获：`Flush` 面对恒脏玩家
// （保存完成前快照又被 `Observe` 更新，保存结果永不等于当前快照）时若不设派发上限会
// 无界重派。本测试钉住方案 A' 的双类名额行为：同一玩家在单次 `Flush` 内 fresh 类至多
// 派发一次，用尽后必须以 `errPlayerFlushStalled` 带错误返回而不是继续重派或静默吞掉
// 残余脏状态；随后残余脏状态与最新快照必须完整保留，供下一次 `Flush` 补发落盘。
func TestPlayerFlushStallReturnsBoundedErrorAndKeepsDirty(t *testing.T) {
	store := newControllablePlayerStore()
	id := playerID(40)
	store.loaded[id] = storedPlayerForTest(id, 7, "A", testPlayerSnapshot(3))
	persistence := newPlayerPersistence(store, playerPersistenceTestConfig())
	t.Cleanup(persistence.CloseWorker)
	if _, err := persistence.Prepare(context.Background(), id, "A", testMetadata()); err != nil {
		t.Fatal(err)
	}
	if err := persistence.Observe(id, "A", testPlayerSnapshot(10), 0, false); err != nil {
		t.Fatal(err)
	}

	flushed := make(chan error, 1)
	go func() { flushed <- persistence.Flush(context.Background()) }()
	first := receivePlayerSave(t, store)
	if first.Revision != 8 {
		t.Fatalf("first save revision=%d, want 8", first.Revision)
	}
	// 保存结果送达前再次 Observe 出一个不同快照：制造「保存结果永不等于当前快照」的恒脏态，
	// 这正是旧实现精确键去重失效、无界重派的触发条件。
	if err := persistence.Observe(id, "A", testPlayerSnapshot(20), 0, false); err != nil {
		t.Fatal(err)
	}
	store.complete(nil)

	var err error
	select {
	case err = <-flushed:
	case <-time.After(waitDeadline):
		t.Fatal("stalled Flush did not return")
	}
	if err == nil || !errors.Is(err, errPlayerFlushStalled) {
		t.Fatalf("Flush error=%v, want errPlayerFlushStalled", err)
	}
	wantText := "save player " + id.String() + " revision 9: " + errPlayerFlushStalled.Error()
	if err.Error() != wantText {
		t.Fatalf("Flush error text=%q, want %q", err.Error(), wantText)
	}
	// 本次 Flush 只应派发过 1 次 save（revision 8）；fresh 名额用尽后不得追派 revision 9。
	assertNoPlayerSaveStarted(t, store)

	// 失速不丢数据：脏状态与最新快照被完整保留，下一次 Flush 必须把它落盘。
	secondFlush := make(chan error, 1)
	go func() { secondFlush <- persistence.Flush(context.Background()) }()
	fresh := receivePlayerSave(t, store)
	if fresh.Revision != 9 || fresh.Current.Position != [3]float32{20, 70, -20} {
		t.Fatalf("post-stall save=%+v, want revision 9 with latest snapshot", fresh)
	}
	store.complete(nil)
	select {
	case err := <-secondFlush:
		if err != nil {
			t.Fatalf("Flush after stall error=%v", err)
		}
	case <-time.After(waitDeadline):
		t.Fatal("Flush after stall did not return")
	}
}
