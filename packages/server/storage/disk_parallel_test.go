package storage

// 本文件钉住世界流式规格的两条 Requirement：「磁盘存档 I/O 按类别与 region
// 并行」与「region 句柄有界且淘汰安全」。断言只落在可观测行为上：文件字节、
// 读取内容、完成顺序与句柄数，不感知内部锁实现。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/server/storage/chunk"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// parallelChunkFor 生成由（坐标, salt）唯一确定的区块：并发等价与淘汰一致性
// 断言都以它的内容哈希为承重取值。
func parallelChunkFor(pos core.ChunkPos, salt int) *world.Chunk {
	chunk := world.NewChunk(pos)
	chunk.SetBlock(salt%16, int32(salt/3%128), (salt*7)%16, core.BlockID(salt%40+1))
	return chunk
}

// gatedSyncRegionFile 把 Region.Save 内的 fsync 变成可停靠的门闩：首个（以及
// 后续每个）Sync 在 release 关闭前阻塞，用于把一次区块保存停在中途。
type gatedSyncRegionFile struct {
	chunk.File
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (file *gatedSyncRegionFile) Sync() error {
	file.once.Do(func() { close(file.started) })
	<-file.release
	return file.File.Sync()
}

// TestDiskStoreConcurrentInterleavingMatchesSerialResult 覆盖 Scenario
// 「并行交错等价于串行结果」：每个 region 携带固定的保存/读取步骤序并发执行，
// 等价串行序即「逐 region 依序应用」；断言磁盘 region 文件逐位一致，且并发
// 期间的每次读取都与该串行序中的某个提交点一致。
func TestDiskStoreConcurrentInterleavingMatchesSerialResult(t *testing.T) {
	const regionCount = 6
	const steps = 8
	const masterSeed = 20260909

	type regionScript struct {
		key    RegionKey
		chunks [2]core.ChunkKey
	}
	scripts := make([]regionScript, regionCount)
	for index := range scripts {
		base := core.ChunkPos{X: int32(index) * 96, Z: int32(index) * -64}
		first := core.ChunkKey{Dimension: core.Overworld, Pos: base}
		second := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: base.X + 5, Z: base.Z + 7}}
		key, _ := RegionFor(first)
		if other, _ := RegionFor(second); other != key {
			t.Fatalf("夹具区块 %v/%v 不属于同一 region", first, second)
		}
		scripts[index] = regionScript{key: key, chunks: [2]core.ChunkKey{first, second}}
	}

	runScript := func(store *DiskStore, script regionScript, jitter *rand.Rand) error {
		for step := 1; step <= steps; step++ {
			if jitter != nil {
				time.Sleep(time.Duration(jitter.Intn(120)) * time.Microsecond)
			}
			saves := make([]ChunkSave, 0, len(script.chunks))
			for _, chunkKey := range script.chunks {
				saves = append(saves, ChunkSave{
					Key: chunkKey, Revision: uint64(step),
					Chunk: parallelChunkFor(chunkKey.Pos, step*31+int(chunkKey.Pos.X&0xf)),
				})
			}
			if _, err := store.SaveBatch(context.Background(), saves); err != nil {
				return fmt.Errorf("save region %+v step %d: %w", script.key, step, err)
			}
			chunkKey := script.chunks[step%len(script.chunks)]
			stored, err := store.LoadChunk(context.Background(), chunkKey)
			if err != nil {
				return fmt.Errorf("load %v after step %d: %w", chunkKey, step, err)
			}
			if stored.Revision != uint64(step) {
				return fmt.Errorf("load %v after step %d: revision %d", chunkKey, step, stored.Revision)
			}
			want := parallelChunkFor(chunkKey.Pos, step*31+int(chunkKey.Pos.X&0xf))
			if stored.Chunk.Hash() != want.Hash() {
				return fmt.Errorf("load %v after step %d: content mismatch", chunkKey, step)
			}
		}
		return nil
	}

	serialRoot := t.TempDir()
	serialStore, err := OpenDisk(context.Background(), serialRoot, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range scripts {
		if err := runScript(serialStore, script, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := serialStore.Close(); err != nil {
		t.Fatal(err)
	}

	concurrentRoot := t.TempDir()
	concurrentStore, err := OpenDisk(context.Background(), concurrentRoot, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatal(err)
	}

	failures := make(chan error, regionCount+1)
	var regionWg sync.WaitGroup
	for index := range scripts {
		regionWg.Add(1)
		go func(index int) {
			defer regionWg.Done()
			jitter := rand.New(rand.NewSource(masterSeed + int64(index) + 1))
			if err := runScript(concurrentStore, scripts[index], jitter); err != nil {
				failures <- err
			}
		}(index)
	}
	roverStop := make(chan struct{})
	var roverWg sync.WaitGroup
	roverWg.Add(1)
	go func() {
		defer roverWg.Done()
		jitter := rand.New(rand.NewSource(masterSeed))
		for {
			select {
			case <-roverStop:
				return
			default:
			}
			script := scripts[jitter.Intn(len(scripts))]
			chunkKey := script.chunks[jitter.Intn(len(script.chunks))]
			stored, err := concurrentStore.LoadChunk(context.Background(), chunkKey)
			switch {
			case err == nil:
				// 每次读取必须与某种串行序一致：revision 是已提交步骤之一且
				// 内容与该步骤的区块逐位同哈希。
				if stored.Revision < 1 || stored.Revision > steps {
					failures <- fmt.Errorf("rover load %v: revision %d 超出提交范围", chunkKey, stored.Revision)
					return
				}
				want := parallelChunkFor(chunkKey.Pos, int(stored.Revision)*31+int(chunkKey.Pos.X&0xf))
				if stored.Chunk.Hash() != want.Hash() {
					failures <- fmt.Errorf("rover load %v: revision %d 内容不匹配", chunkKey, stored.Revision)
					return
				}
			case errors.Is(err, ErrChunkNotFound):
				// 首次保存尚未发生时允许未找到。
			default:
				failures <- fmt.Errorf("rover load %v: %w", chunkKey, err)
				return
			}
			time.Sleep(60 * time.Microsecond)
		}
	}()
	regionWg.Wait()
	close(roverStop)
	roverWg.Wait()
	if err := concurrentStore.Close(); err != nil {
		t.Fatal(err)
	}
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}

	// 磁盘上的全部 region 文件与串行执行逐位一致。
	for _, script := range scripts {
		serialBytes, err := os.ReadFile(filepath.Join(serialRoot, dimensionRegionPath(script.key)))
		if err != nil {
			t.Fatal(err)
		}
		concurrentBytes, err := os.ReadFile(filepath.Join(concurrentRoot, dimensionRegionPath(script.key)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(serialBytes, concurrentBytes) {
			t.Fatalf("region %+v 并发结果与串行结果不一致", script.key)
		}
	}
}

// TestDiskStoreConcurrentSavesOnSameRegionSerializeAndStayMonotone 覆盖
// Scenario「同一 region 内保持串行」：同 region 的并发保存逐次提交，文件始终
// 能解码出合法 bank，读取到的 revision 单调不减且最终收敛到最高提交。
func TestDiskStoreConcurrentSavesOnSameRegionSerializeAndStayMonotone(t *testing.T) {
	const rounds = 10
	root := t.TempDir()
	store, err := OpenDisk(context.Background(), root, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 9, Z: -13}}
	stop := make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		defer func() { readerDone <- nil }()
		last := uint64(0)
		for {
			select {
			case <-stop:
				return
			default:
			}
			stored, err := store.LoadChunk(context.Background(), key)
			if err == nil {
				if stored.Revision < last {
					readerDone <- fmt.Errorf("并发读取 revision 回退：%d < %d", stored.Revision, last)
					return
				}
				last = stored.Revision
				want := parallelChunkFor(key.Pos, int(stored.Revision)*17+3)
				if stored.Chunk.Hash() != want.Hash() {
					readerDone <- fmt.Errorf("并发读取 revision %d 内容损坏", stored.Revision)
					return
				}
			} else if !errors.Is(err, ErrChunkNotFound) {
				readerDone <- fmt.Errorf("并发读取：%w", err)
				return
			}
			time.Sleep(40 * time.Microsecond)
		}
	}()

	for round := 1; round <= rounds; round++ {
		lower := ChunkSave{
			Key: key, Revision: uint64(2*round - 1),
			Chunk: parallelChunkFor(key.Pos, (2*round-1)*17+3),
		}
		higher := ChunkSave{
			Key: key, Revision: uint64(2 * round),
			Chunk: parallelChunkFor(key.Pos, (2*round)*17+3),
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var pair sync.WaitGroup
		for _, save := range []ChunkSave{lower, higher} {
			pair.Add(1)
			go func(save ChunkSave) {
				defer pair.Done()
				<-start
				if _, err := store.SaveBatch(context.Background(), []ChunkSave{save}); err != nil {
					results <- fmt.Errorf("并发保存 revision %d：%w", save.Revision, err)
				}
			}(save)
		}
		close(start)
		pair.Wait()
		close(results)
		for failure := range results {
			t.Fatal(failure)
		}
		stored, err := store.LoadChunk(context.Background(), key)
		if err != nil || stored.Revision != uint64(2*round) {
			t.Fatalf("round %d 后 revision = %d（err %v），想要 %d", round, stored.Revision, err, 2*round)
		}
	}

	close(stop)
	if err := <-readerDone; err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// 解码恒可选出合法 bank：重开目录后全部数据仍可读取且 revision 收敛。
	reopened, err := OpenDisk(context.Background(), root, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err := reopened.LoadChunk(context.Background(), key)
	if err != nil || stored.Revision != 2*rounds {
		t.Fatalf("重开后 revision = %d（err %v），想要 %d", stored.Revision, err, 2*rounds)
	}
	want := parallelChunkFor(key.Pos, (2*rounds)*17+3)
	if stored.Chunk.Hash() != want.Hash() {
		t.Fatal("重开后内容与最终提交不一致")
	}
}

// TestDiskStoreCategorySavesCompleteWhileChunkBatchParked 覆盖 Scenario
// 「类别间互不阻塞」：区块批次停靠在 fsync 门闩上时，其余五类存档保存全部
// 可以完成（结构性完成顺序断言，不依赖时序）。
func TestDiskStoreCategorySavesCompleteWhileChunkBatchParked(t *testing.T) {
	store := openTestDiskStore(t)
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 3, Z: 4}}
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, 1)); err != nil {
		t.Fatal(err)
	}
	regionKey, _ := RegionFor(key)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	// defer 逆序保证失败路径先放行门闩再 Close，避免 Close 排空时卡死。
	defer store.Close()
	defer releaseOnce.Do(func() { close(release) })
	handle := store.regions[regionKey]
	handle.ReplaceFile(&gatedSyncRegionFile{
		File: handle.File(), started: started, release: release,
	})

	batchDone := make(chan error, 1)
	go func() {
		_, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, 2))
		batchDone <- err
	}()
	waitForTestSignal(t, started, "区块批次未到达 fsync 门闩")

	ctx := context.Background()
	categoryDone := make(chan error, 5)
	go func() {
		if _, err := store.SavePlayer(ctx, fixturePlayerSave(fixturePlayerID(), 1)); err != nil {
			categoryDone <- fmt.Errorf("SavePlayer: %w", err)
		} else {
			categoryDone <- nil
		}
	}()
	go func() {
		save := fixtureCompanionV5Save(CompanionSave{Revision: 1, Records: fixtureCompanionBodies()})
		categoryDone <- store.SaveCompanions(ctx, save)
	}()
	go func() {
		categoryDone <- store.SaveHostileMobs(ctx, HostileMobsSave{Revision: 1, Records: fixtureHostileRecords()})
	}()
	go func() {
		categoryDone <- store.SavePassiveMobs(ctx, PassiveMobsSave{Revision: 1, Records: fixturePassiveRecords()})
	}()
	go func() {
		categoryDone <- store.SaveMetadata(ctx, Metadata{FormatVersion: currentMetadataVersion, Seed: 4242})
	}()
	for index := 0; index < 5; index++ {
		if err := waitForTestError(t, categoryDone, "存档类别保存被停靠的区块批次阻塞"); err != nil {
			t.Fatal(err)
		}
	}

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, batchDone, "停靠的区块批次未完成"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadChunk(ctx, key)
	if err != nil || stored.Revision != 2 {
		t.Fatalf("停靠后区块 revision = %d（err %v），想要 2", stored.Revision, err)
	}
}

// TestDiskStoreChunkIOCompletesWhilePlayerSaveParked 钉住类别不阻塞的方向对称：
// 玩家保存停靠在临时文件创建钩子上时，区块保存、读取与 ChunkKeys 照常完成。
func TestDiskStoreChunkIOCompletesWhilePlayerSaveParked(t *testing.T) {
	store := openTestDiskStore(t)
	defer store.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	store.playerReplaceHooks = atomicReplaceHooks{
		createTemp: func(directory, pattern string) (atomicReplaceFile, error) {
			close(started)
			<-release
			return os.CreateTemp(directory, pattern)
		},
	}

	playerDone := make(chan error, 1)
	go func() {
		_, err := store.SavePlayer(context.Background(), fixturePlayerSave(fixturePlayerID(), 1))
		playerDone <- err
	}()
	waitForTestSignal(t, started, "玩家保存未到达临时文件门闩")

	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 21, Z: -3}}
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, 1)); err != nil {
		t.Fatalf("停靠玩家保存期间的区块保存：%v", err)
	}
	if _, err := store.LoadChunk(context.Background(), key); err != nil {
		t.Fatalf("停靠玩家保存期间的区块读取：%v", err)
	}
	if _, err := store.ChunkKeys(context.Background()); err != nil {
		t.Fatalf("停靠玩家保存期间的 ChunkKeys：%v", err)
	}

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, playerDone, "停靠的玩家保存未完成"); err != nil {
		t.Fatal(err)
	}
}

// TestDiskStoreRegionHandlesStayBoundedAndEvictedRegionsReopenConsistently 覆盖
// Scenario「句柄数受上限约束」与「被淘汰的 region 再次被访问时数据一致」：
// 依序访问远超上限的 region，任一时刻缓存句柄数不超过上限，全部读写成功且
// 淘汰后的再次访问返回与未淘汰时一致的数据。
func TestDiskStoreRegionHandlesStayBoundedAndEvictedRegionsReopenConsistently(t *testing.T) {
	const handleCap = 4
	const regionCount = 10
	const rounds = 3
	root := t.TempDir()
	store, err := OpenDisk(context.Background(), root, OpenOptions{
		Create:               Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
		RegionHandleCacheCap: handleCap,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	keyFor := func(index int) core.ChunkKey {
		return core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: int32(index) * 32, Z: 17}}
	}
	for round := 1; round <= rounds; round++ {
		for index := 0; index < regionCount; index++ {
			key := keyFor(index)
			save := ChunkSave{
				Key: key, Revision: uint64(round),
				Chunk: parallelChunkFor(key.Pos, round*101+index),
			}
			if _, err := store.SaveBatch(context.Background(), []ChunkSave{save}); err != nil {
				t.Fatalf("save region %d round %d: %v", index, round, err)
			}
			if len(store.regions) > handleCap {
				t.Fatalf("round %d region %d 后句柄数 %d 超过上限 %d", round, index, len(store.regions), handleCap)
			}
		}
	}
	for index := 0; index < regionCount; index++ {
		key := keyFor(index)
		stored, err := store.LoadChunk(context.Background(), key)
		if err != nil {
			t.Fatalf("load region %d: %v", index, err)
		}
		if stored.Revision != rounds {
			t.Fatalf("load region %d: revision %d, want %d", index, stored.Revision, rounds)
		}
		want := parallelChunkFor(key.Pos, rounds*101+index)
		if stored.Chunk.Hash() != want.Hash() {
			t.Fatalf("load region %d: 淘汰后数据与最终提交不一致", index)
		}
	}
	if len(store.regions) > handleCap {
		t.Fatalf("读取后句柄数 %d 超过上限 %d", len(store.regions), handleCap)
	}
}

// TestDiskStoreInFlightRegionReferenceDefersEviction 覆盖 Scenario「在途引用
// 不被淘汰关闭」：保存停靠在 fsync 门闩上时反复触发淘汰，保存仍正常完成；
// 引用归还后缓存收缩回上限内。
func TestDiskStoreInFlightRegionReferenceDefersEviction(t *testing.T) {
	const handleCap = 2
	root := t.TempDir()
	store, err := OpenDisk(context.Background(), root, OpenOptions{
		Create:               Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
		RegionHandleCacheCap: handleCap,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	keyFor := func(index int) core.ChunkKey {
		return core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: int32(index) * 32, Z: -5}}
	}
	parked := keyFor(0)
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{parked}, 1)); err != nil {
		t.Fatal(err)
	}
	regionKey, _ := RegionFor(parked)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	handle := store.regions[regionKey]
	handle.ReplaceFile(&gatedSyncRegionFile{
		File: handle.File(), started: started, release: release,
	})

	saveDone := make(chan error, 1)
	go func() {
		_, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{parked}, 2))
		saveDone <- err
	}()
	waitForTestSignal(t, started, "在途保存未到达 fsync 门闩")

	// 停靠期间访问远超上限的其他 region，反复触发对在途句柄的淘汰尝试。
	for index := 1; index <= 6; index++ {
		if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{keyFor(index)}, 1)); err != nil {
			t.Fatalf("停靠期间 save region %d: %v", index, err)
		}
		// 窗口硬上界：停靠的保存持有唯一在途引用，句柄数不得超过「上限加
		// 在途引用数」，空闲候选必须照常被淘汰回收。
		if bound := handleCap + 1; len(store.regions) > bound {
			t.Fatalf("停靠期间句柄数 %d 超过上限 %d 加在途引用 1 的上界 %d", len(store.regions), handleCap, bound)
		}
	}

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, saveDone, "在途保存未正常完成"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadChunk(context.Background(), parked)
	if err != nil || stored.Revision != 2 {
		t.Fatalf("在途保存结果 revision = %d（err %v），想要 2", stored.Revision, err)
	}

	// 引用归还后，下一次缓存治理把句柄数收回上限内。
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{keyFor(7)}, 1)); err != nil {
		t.Fatal(err)
	}
	if len(store.regions) > handleCap {
		t.Fatalf("引用归还后句柄数 %d 超过上限 %d", len(store.regions), handleCap)
	}
}

// TestDiskStoreChunkKeysRunsWhileChunkSaveParkedAndBackupSerializes 钉住全量
// 枚举与备份复制在并行边界上的分工：区块保存停靠（持容器写锁）期间
// ChunkKeys 照常完成（独立临时打开，不经缓存句柄、不持 region 锁）；Backup
// 的 region 文件复制经缓存句柄读锁与停靠保存串行——停靠期间不可能完成，
// 保存提交后完成的备份与在线文件逐位一致（复制总是完整提交点）。
func TestDiskStoreChunkKeysRunsWhileChunkSaveParkedAndBackupSerializes(t *testing.T) {
	root := t.TempDir()
	store, err := OpenDisk(context.Background(), root, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	parked := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 6, Z: 8}}
	other := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 70, Z: -70}}
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{parked, other}, 1)); err != nil {
		t.Fatal(err)
	}
	parkedRegion, _ := RegionFor(parked)
	otherRegion, _ := RegionFor(other)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	// defer 逆序保证失败路径先放行门闩再 Close，避免 Close 排空时卡死。
	defer store.Close()
	defer releaseOnce.Do(func() { close(release) })
	handle := store.regions[parkedRegion]
	handle.ReplaceFile(&gatedSyncRegionFile{
		File: handle.File(), started: started, release: release,
	})

	saveDone := make(chan error, 1)
	go func() {
		_, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{parked}, 2))
		saveDone <- err
	}()
	waitForTestSignal(t, started, "区块保存未到达 fsync 门闩")

	keysDone := make(chan error, 1)
	go func() {
		_, err := store.ChunkKeys(context.Background())
		keysDone <- err
	}()
	if err := waitForTestError(t, keysDone, "ChunkKeys 被停靠的区块保存阻塞"); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "backup")
	backupDone := make(chan error, 1)
	go func() {
		backupDone <- store.Backup(context.Background(), destination)
	}()
	// 停靠保存持写锁：备份对停靠 region 的复制被读锁串行化，不可能完成。
	assertNotCompletedWithin(t, backupDone, 150*time.Millisecond, "停靠的区块保存持写锁期间 Backup 已完成")

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, saveDone, "停靠的区块保存未完成"); err != nil {
		t.Fatal(err)
	}
	if err := waitForTestError(t, backupDone, "串行后的 Backup 未完成"); err != nil {
		t.Fatal(err)
	}
	// 备份内的 region 文件分别在各自提交点之后复制，与在线文件逐位一致。
	assertSameFileContents(
		t,
		filepath.Join(root, dimensionRegionPath(parkedRegion)),
		filepath.Join(destination, dimensionRegionPath(parkedRegion)),
	)
	assertSameFileContents(
		t,
		filepath.Join(root, dimensionRegionPath(otherRegion)),
		filepath.Join(destination, dimensionRegionPath(otherRegion)),
	)
	stored, err := store.LoadChunk(context.Background(), parked)
	if err != nil || stored.Revision != 2 {
		t.Fatalf("停靠后区块 revision = %d（err %v），想要 2", stored.Revision, err)
	}
}

// dimensionRegionPath 返回 region 文件相对世界根的路径，供逐位比较直接读档。
func dimensionRegionPath(key RegionKey) string {
	return filepath.Join(
		"dimensions", fmt.Sprintf("%d", key.Dimension),
		"regions", fmt.Sprintf("r.%d.%d.region", key.X, key.Z),
	)
}
