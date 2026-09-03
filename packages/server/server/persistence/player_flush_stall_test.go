package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
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
	persistence := NewPlayers(store, playerPersistenceTestConfig())
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

// TestPlayerFlushStallOnlyReportsStalledPlayerAlongsideExistingFailure 钉住 spec 的
// 「已有失败只报原错误」Scenario：同一次 Flush 内玩家 A 的保存失败并冻结 retry（本次
// Flush 已记录失败），玩家 B 恒脏失速。断言最终错误里 A 只出现一次原始失败文本、不被
// `recordFlushStallLocked` 追加 `errPlayerFlushStalled`，B 则恰好计入一条失速错误——
// 二者互不覆盖、互不影响对方在 `failures` 里的记录。
func TestPlayerFlushStallOnlyReportsStalledPlayerAlongsideExistingFailure(t *testing.T) {
	store := newControllablePlayerStore()
	idA, idB := playerID(41), playerID(42) // idA 字节序小于 idB，主循环按 ID 升序先选中 A。
	store.loaded[idA] = storedPlayerForTest(idA, 7, "A", testPlayerSnapshot(3))
	store.loaded[idB] = storedPlayerForTest(idB, 7, "B", testPlayerSnapshot(4))
	persistence := NewPlayers(store, playerPersistenceTestConfig())
	t.Cleanup(persistence.CloseWorker)
	for _, setup := range []struct {
		id   core.PlayerID
		name string
	}{{idA, "A"}, {idB, "B"}} {
		if _, err := persistence.Prepare(context.Background(), setup.id, setup.name, testMetadata()); err != nil {
			t.Fatal(err)
		}
		if err := persistence.Observe(setup.id, setup.name, testPlayerSnapshot(10), 0, false); err != nil {
			t.Fatal(err)
		}
	}

	flushed := make(chan error, 1)
	go func() { flushed <- persistence.Flush(context.Background()) }()

	// 主循环按 ID 升序单选一个候选派发：A 先被选中（fresh，revision 8），失败后冻结
	// retry；A 的 retry 名额随之占满，下一轮候选让给 B。
	firstSave := receivePlayerSave(t, store)
	if firstSave.PlayerID != idA || firstSave.Revision != 8 {
		t.Fatalf("first save=%+v, want player A revision 8", firstSave)
	}
	wantErr := errors.New("disk full")
	store.complete(wantErr)

	// B 被选中派发（fresh，revision 8）；完成前再 Observe 一个不同快照制造恒脏，
	// B 的 fresh 名额随完成而用尽，之后不再具备可派发的候选。
	secondSave := receivePlayerSave(t, store)
	if secondSave.PlayerID != idB || secondSave.Revision != 8 {
		t.Fatalf("second save=%+v, want player B revision 8", secondSave)
	}
	if err := persistence.Observe(idB, "B", testPlayerSnapshot(20), 0, false); err != nil {
		t.Fatal(err)
	}
	store.complete(nil)

	var err error
	select {
	case err = <-flushed:
	case <-time.After(waitDeadline):
		t.Fatal("mixed-stall Flush did not return")
	}
	wantText := "save player " + idA.String() + " revision 8: " + wantErr.Error() + "\n" +
		"save player " + idB.String() + " revision 9: " + errPlayerFlushStalled.Error()
	if err == nil || err.Error() != wantText {
		t.Fatalf("Flush error=%q, want %q", err, wantText)
	}
	if !errors.Is(err, wantErr) || !errors.Is(err, errPlayerFlushStalled) {
		t.Fatalf("Flush error=%v does not retain both roots", err)
	}
	assertNoPlayerSaveStarted(t, store)
}
