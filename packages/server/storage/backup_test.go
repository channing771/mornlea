package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestWorldBackupCopiesCompleteWorldAndReusesMatchingBackup(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)

	temporaryPaths := []string{
		filepath.Join(source, ".world.meta.tmp-ignore"),
		filepath.Join(source, "players", ".player.tmp-ignore"),
		filepath.Join(source, "dimensions", ".scan.tmp-ignore", "entry"),
		// CreateRegion 落盘 region 前经 `.create-*` 临时文件原子提交，备份过滤
		// 必须与其余临时命名同等跳过半写的 region 候选。
		filepath.Join(source, "dimensions", "0", "regions", ".r.-2.1.region.create-ignore"),
	}
	for _, path := range temporaryPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("创建临时条目目录失败: %v", err)
		}
		if err := os.WriteFile(path, []byte("temporary"), 0o600); err != nil {
			t.Fatalf("写入临时条目失败: %v", err)
		}
	}

	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatalf("创建世界备份失败: %v", err)
	}

	for _, relativePath := range []string{
		"world.meta",
		filepath.Join("players", "player-one.player"),
		filepath.Join("dimensions", "0", "regions", "r.-2.1.region"),
	} {
		assertSameFileContents(t, filepath.Join(source, relativePath), filepath.Join(destination, relativePath))
	}
	for _, relativePath := range []string{
		"world.lock",
		".world.meta.tmp-ignore",
		filepath.Join("players", ".player.tmp-ignore"),
		filepath.Join("dimensions", ".scan.tmp-ignore"),
		filepath.Join("dimensions", "0", "regions", ".r.-2.1.region.create-ignore"),
	} {
		if _, err := os.Lstat(filepath.Join(destination, relativePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("备份不应包含 %q，Lstat 错误: %v", relativePath, err)
		}
	}

	identityData, err := os.ReadFile(filepath.Join(destination, ".mcgo-world-backup-v1.json"))
	if err != nil {
		t.Fatalf("读取备份身份失败: %v", err)
	}
	var identity struct {
		Source           string `json:"source"`
		Seed             int64  `json:"seed"`
		MigrationVersion int    `json:"migration_version"`
	}
	if err := json.Unmarshal(identityData, &identity); err != nil {
		t.Fatalf("解析备份身份失败: %v", err)
	}
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		t.Fatalf("规范源路径失败: %v", err)
	}
	wantIdentity := struct {
		Source           string `json:"source"`
		Seed             int64  `json:"seed"`
		MigrationVersion int    `json:"migration_version"`
	}{
		Source:           absoluteSource,
		Seed:             42,
		MigrationVersion: 1,
	}
	if identity != wantIdentity {
		t.Fatalf("备份身份 = %+v，期望 %+v", identity, wantIdentity)
	}

	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatalf("复用身份匹配的世界备份失败: %v", err)
	}
}

func TestWorldBackupIncludesCompanionFileButSkipsTemporaryFiles(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	if err := store.SaveCompanions(context.Background(), fixtureCompanionV5Save(CompanionSave{
		Revision: 1, Records: fixtureCompanionBodies(),
	})); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(source, ".companions.ai.tmp-ignore")
	if err := os.WriteFile(temporary, []byte("temporary"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	assertSameFileContents(
		t, filepath.Join(source, "companions.ai"), filepath.Join(destination, "companions.ai"),
	)
	if _, err := os.Lstat(filepath.Join(destination, filepath.Base(temporary))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("备份不应包含伙伴临时文件，Lstat 错误: %v", err)
	}
}

// TestWorldBackupDuringParkedCompactRenameSkipsTemporaryFiles 钉住 Backup 与
// 并发 Compact 的交错安全：Compact 的临时替换文件停靠在 rename（持容器写
// 锁）时，Backup 的 region 复制经读锁与该提交串行——停靠期间 Backup 不可能
// 完成，放行后 Backup 成功且备份目录不含任何 compact/temp 残留（半写临时
// 文件不得混入备份），正式 region 文件随提交结果进入备份。
func TestWorldBackupDuringParkedCompactRenameSkipsTemporaryFiles(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	key := core.ChunkKey{Dimension: 0, Pos: core.ChunkPos{X: -33, Z: 34}}
	regionKey, _ := RegionFor(key)
	handle := store.regions[regionKey]
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	// defer 逆序保证失败路径先放行门闩再执行夹具的 Close 清理，Close 在
	// region 互斥上等待停靠的 Compact 释放，避免清理卡死。
	defer releaseOnce.Do(func() { close(release) })
	handle.SetCompactionHooks(region.CompactionHooks{
		Rename: func(temporary, canonical string) error {
			close(started)
			<-release
			return os.Rename(temporary, canonical)
		},
	})

	compactDone := make(chan error, 1)
	go func() {
		compactDone <- handle.Compact(context.Background())
	}()
	waitForTestSignal(t, started, "Compact 未到达 rename 门闩")

	backupDone := make(chan error, 1)
	go func() {
		backupDone <- store.Backup(context.Background(), destination)
	}()
	// Compact 持写锁停靠：备份对该 region 的复制被读锁串行化，不可能完成。
	assertNotCompletedWithin(t, backupDone, 150*time.Millisecond, "停靠的 Compact 持写锁期间 Backup 已完成")

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, compactDone, "停靠的 Compact 未完成"); err != nil {
		t.Fatal(err)
	}
	if err := waitForTestError(t, backupDone, "串行后的 Backup 未完成"); err != nil {
		t.Fatal(err)
	}
	assertBackupFreeOfTemporaryResidue(t, destination)
	if _, err := os.Lstat(filepath.Join(destination, dimensionRegionPath(regionKey))); err != nil {
		t.Fatalf("备份缺少正式 region 文件: %v", err)
	}

	stored, err := store.LoadChunk(context.Background(), key)
	if err != nil || stored.Revision != 1 {
		t.Fatalf("Compact 完成后读取 revision = %d（err %v），想要 1", stored.Revision, err)
	}
}

// gatedReadRegionFile（定义于 disk_test.go）把备份复制期间的 region 读取
// 变成可停靠的门闩：首个 ReadAt 在 release 关闭前阻塞，把一次备份复制停在
// 第一个读取上（复制经容器读锁进行，停靠期间读锁仍被持有），后续读取直通。

// assertNotCompletedWithin 断言 result 通道在窗口内保持未完成：用于门闩
// 持有互斥期间的结构性阻塞证明（被阻塞的操作不可能完成，窗口只是观察
// 时长）。
func assertNotCompletedWithin(t *testing.T, result <-chan error, window time.Duration, failure string) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("%s: %v", failure, err)
	case <-time.After(window):
	}
}

// TestWorldBackupRegionCopyBlocksConcurrentSaveOnSameRegion 钉住备份复制与
// 并发保存的串行化：region 文件的复制经缓存句柄持容器读锁，复制停靠在读取
// 中途（读锁仍持有）时，同 region 的保存必须保持未完成直至复制结束。复制
// 若不经缓存句柄而直读文件（修复前形态），本测试在「备份复制未到达读取
// 门闩」处即失败。
func TestWorldBackupRegionCopyBlocksConcurrentSaveOnSameRegion(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	key := core.ChunkKey{Dimension: 0, Pos: core.ChunkPos{X: -33, Z: 34}}
	regionKey, _ := RegionFor(key)
	handle := store.regions[regionKey]
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	// defer 逆序先放行门闩再执行夹具的 Close 清理：Close 排空在途引用时
	// 复制必须已可继续推进，避免清理卡死。
	defer releaseOnce.Do(func() { close(release) })
	handle.ReplaceFile(&gatedReadRegionFile{
		File: handle.File(), started: started, release: release,
	})

	backupDone := make(chan error, 1)
	go func() {
		backupDone <- store.Backup(context.Background(), destination)
	}()
	waitForTestSignal(t, started, "备份复制未到达 region 读取门闩")

	saveDone := make(chan error, 1)
	go func() {
		_, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, 2))
		saveDone <- err
	}()
	assertNotCompletedWithin(t, saveDone, 150*time.Millisecond, "复制持读锁期间同 region 保存已完成")

	releaseOnce.Do(func() { close(release) })
	if err := waitForTestError(t, saveDone, "停靠复制释放后保存未完成"); err != nil {
		t.Fatal(err)
	}
	if err := waitForTestError(t, backupDone, "停靠的备份未完成"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadChunk(context.Background(), key)
	if err != nil || stored.Revision != 2 {
		t.Fatalf("保存完成后 revision = %d（err %v），想要 2", stored.Revision, err)
	}
}

// TestWorldBackupWithConcurrentSavesDuringCopyYieldsCommittedRevision 端到端
// 验证备份成品的可解码性：备份复制停靠在 region 读取中途时并发发起多次
// 同 region 保存（全部排队在容器写锁之后），复制完成后保存逐次提交；备份
// 副本重开后必须能读出复制开始前已提交的 revision 与内容，在线世界收敛到
// 最终 revision。
func TestWorldBackupWithConcurrentSavesDuringCopyYieldsCommittedRevision(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	key := core.ChunkKey{Dimension: 0, Pos: core.ChunkPos{X: -33, Z: 34}}
	regionKey, _ := RegionFor(key)
	handle := store.regions[regionKey]
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	handle.ReplaceFile(&gatedReadRegionFile{
		File: handle.File(), started: started, release: release,
	})

	backupDone := make(chan error, 1)
	go func() {
		backupDone <- store.Backup(context.Background(), destination)
	}()
	waitForTestSignal(t, started, "备份复制未到达 region 读取门闩")

	const finalRevision = uint64(4)
	savesDone := make(chan error, finalRevision-1)
	for revision := uint64(2); revision <= finalRevision; revision++ {
		go func(revision uint64) {
			_, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, revision))
			savesDone <- err
		}(revision)
	}
	for revision := uint64(2); revision <= finalRevision; revision++ {
		assertNotCompletedWithin(t, savesDone, 150*time.Millisecond, "复制持读锁期间保存已完成")
	}

	releaseOnce.Do(func() { close(release) })
	for revision := uint64(2); revision <= finalRevision; revision++ {
		if err := waitForTestError(t, savesDone, "停靠复制释放后保存未完成"); err != nil {
			t.Fatal(err)
		}
	}
	if err := waitForTestError(t, backupDone, "停靠的备份未完成"); err != nil {
		t.Fatal(err)
	}

	stored, err := store.LoadChunk(context.Background(), key)
	if err != nil || stored.Revision != finalRevision {
		t.Fatalf("在线 revision = %d（err %v），想要 %d", stored.Revision, err, finalRevision)
	}

	backupStore, err := OpenDisk(context.Background(), destination, OpenOptions{})
	if err != nil {
		t.Fatalf("打开备份副本失败: %v", err)
	}
	t.Cleanup(func() {
		if err := backupStore.Close(); err != nil {
			t.Errorf("关闭备份副本失败: %v", err)
		}
	})
	backupChunk, err := backupStore.LoadChunk(context.Background(), key)
	if err != nil || backupChunk.Revision != 1 {
		t.Fatalf("备份副本 revision = %d（err %v），想要复制开始前的 1", backupChunk.Revision, err)
	}
	want := diskSavesFor([]core.ChunkKey{key}, 1)[0].Chunk
	if backupChunk.Chunk.Hash() != want.Hash() {
		t.Fatal("备份副本内容与复制开始前的提交点不一致")
	}
}

func TestWorldBackupCanBackUpAnOpenedBackup(t *testing.T) {
	store, _, firstDestination := newWorldBackupFixture(t)
	if err := store.Backup(context.Background(), firstDestination); err != nil {
		t.Fatalf("创建第一份世界备份失败: %v", err)
	}

	backupStore, err := OpenDisk(context.Background(), firstDestination, OpenOptions{})
	if err != nil {
		t.Fatalf("打开第一份世界备份失败: %v", err)
	}
	t.Cleanup(func() {
		if err := backupStore.Close(); err != nil {
			t.Errorf("关闭第一份世界备份失败: %v", err)
		}
	})
	secondDestination := filepath.Join(filepath.Dir(firstDestination), "second-backup")
	if err := backupStore.Backup(context.Background(), secondDestination); err != nil {
		t.Fatalf("从已打开备份创建第二份备份失败: %v", err)
	}

	identity := readWorldBackupIdentity(t, secondDestination)
	absoluteFirst, err := filepath.Abs(firstDestination)
	if err != nil {
		t.Fatalf("规范第一份备份路径失败: %v", err)
	}
	if identity.Source != absoluteFirst || identity.Seed != 42 || identity.MigrationVersion != 1 {
		t.Fatalf("第二份备份身份 = %+v，期望绑定第一份备份", identity)
	}
}

func TestWorldBackupRejectsEveryMismatchedIdentityField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*backupIdentity)
	}{
		{name: "Source", mutate: func(identity *backupIdentity) { identity.Source += "-other" }},
		{name: "Seed", mutate: func(identity *backupIdentity) { identity.Seed++ }},
		{name: "MigrationVersion", mutate: func(identity *backupIdentity) { identity.MigrationVersion++ }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, destination := newWorldBackupFixture(t)
			if err := store.Backup(context.Background(), destination); err != nil {
				t.Fatalf("创建世界备份失败: %v", err)
			}
			identity := readWorldBackupIdentity(t, destination)
			tc.mutate(&identity)
			mismatched, err := json.Marshal(identity)
			if err != nil {
				t.Fatalf("编码不匹配身份失败: %v", err)
			}
			mismatched = append(mismatched, '\n')
			identityPath := filepath.Join(destination, ".mcgo-world-backup-v1.json")
			if err := os.WriteFile(identityPath, mismatched, 0o600); err != nil {
				t.Fatalf("写入不匹配身份失败: %v", err)
			}
			before := snapshotWorldBackupSource(t, destination)

			if err := store.Backup(context.Background(), destination); err == nil {
				t.Fatalf("%s 不匹配时 Backup() 未返回错误", tc.name)
			}
			after := snapshotWorldBackupSource(t, destination)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("拒绝 %s 不匹配后目标被改变", tc.name)
			}
		})
	}
}

func TestWorldBackupRejectsOversizedIdentity(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	if err := store.Backup(context.Background(), destination); err != nil {
		t.Fatalf("创建世界备份失败: %v", err)
	}
	identityPath := filepath.Join(destination, ".mcgo-world-backup-v1.json")
	identity, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatalf("读取备份身份失败: %v", err)
	}
	oversized := bytes.Repeat([]byte(" "), 4097)
	copy(oversized, identity)
	if err := os.WriteFile(identityPath, oversized, 0o600); err != nil {
		t.Fatalf("写入超限备份身份失败: %v", err)
	}

	if err := store.Backup(context.Background(), destination); err == nil {
		t.Fatal("身份超过 4 KiB 时 Backup() 未返回错误")
	}
	got, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatalf("读取拒绝后的超限身份失败: %v", err)
	}
	if !bytes.Equal(got, oversized) {
		t.Fatal("拒绝超限身份后目标被改变")
	}
}

func TestWorldBackupRetryCompletesParentSyncAfterPublish(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	parent := filepath.Dir(destination)
	injected := errors.New("injected backup parent sync failure")
	failedOnce := false
	parentSyncCompleted := false
	syncDir := func(path string) error {
		if path != parent {
			return syncDirectory(path)
		}
		if !failedOnce {
			failedOnce = true
			return injected
		}
		if err := syncDirectory(path); err != nil {
			return err
		}
		parentSyncCompleted = true
		return nil
	}

	err := store.backup(context.Background(), destination, os.Rename, syncDir)
	if !errors.Is(err, injected) {
		t.Fatalf("首次 Backup() 错误 = %v，期望注入的父目录同步错误", err)
	}
	if parentSyncCompleted {
		t.Fatal("首次失败被错误地记录为已完成父目录同步")
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("rename 成功后的正式备份不可检查: %v", err)
	}

	if err := store.backup(context.Background(), destination, os.Rename, syncDir); err != nil {
		t.Fatalf("重试世界备份失败: %v", err)
	}
	if !parentSyncCompleted {
		t.Fatal("重试静默复用备份，未重新完成父目录同步")
	}
}

func TestWorldBackupRejectsDestinationInsideSource(t *testing.T) {
	store, source, _ := newWorldBackupFixture(t)
	destination := filepath.Join(source, "backup")

	if err := store.Backup(context.Background(), destination); err == nil {
		t.Fatal("目标位于源世界内部时 Backup() 未返回错误")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("拒绝内部目标后目标不应存在，Lstat 错误: %v", err)
	}
}

func TestWorldBackupRejectsDestinationAliasedInsideSource(t *testing.T) {
	store, source, _ := newWorldBackupFixture(t)
	alias := filepath.Join(filepath.Dir(source), "world-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Skipf("当前文件系统不支持符号链接: %v", err)
	}
	destination := filepath.Join(alias, "backup")

	if err := store.Backup(context.Background(), destination); err == nil {
		t.Fatal("目标经符号链接指向源世界内部时 Backup() 未返回错误")
	}
	if _, err := os.Lstat(filepath.Join(source, "backup")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("拒绝路径别名后目标不应存在，Lstat 错误: %v", err)
	}
}

func TestWorldBackupRejectsSymlink(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	target := filepath.Join(filepath.Dir(source), "outside")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatalf("写入符号链接目标失败: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(source, "players", "linked.player")); err != nil {
		t.Skipf("当前文件系统不支持符号链接: %v", err)
	}

	if err := store.Backup(context.Background(), destination); err == nil {
		t.Fatal("源世界含符号链接时 Backup() 未返回错误")
	}
	assertWorldBackupAbsent(t, destination)
	if got, err := os.ReadFile(target); err != nil || string(got) != "outside" {
		t.Fatalf("符号链接目标被改变: data=%q err=%v", got, err)
	}
}

func TestWorldBackupRejectsExistingOrdinaryDirectory(t *testing.T) {
	store, _, destination := newWorldBackupFixture(t)
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatalf("创建既有目标目录失败: %v", err)
	}
	bystander := filepath.Join(destination, "keep")
	if err := os.WriteFile(bystander, []byte("keep"), 0o600); err != nil {
		t.Fatalf("写入既有目标文件失败: %v", err)
	}

	if err := store.Backup(context.Background(), destination); err == nil {
		t.Fatal("既有普通目录缺少匹配身份时 Backup() 未返回错误")
	}
	if got, err := os.ReadFile(bystander); err != nil || string(got) != "keep" {
		t.Fatalf("既有目标目录被改变: data=%q err=%v", got, err)
	}
}

func TestWorldBackupCancellationLeavesSourceUnchanged(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	before := snapshotWorldBackupSource(t, source)
	bystander := filepath.Join(filepath.Dir(destination), ".backup.tmp-keep")
	if err := os.Mkdir(bystander, 0o755); err != nil {
		t.Fatalf("创建旁观临时目录失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Backup(ctx, destination); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消 Backup() 错误 = %v，期望 context.Canceled", err)
	}

	after := snapshotWorldBackupSource(t, source)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("取消备份改变了源世界\nBefore: %#v\nAfter:  %#v", before, after)
	}
	assertWorldBackupAbsent(t, destination)
	if _, err := os.Stat(bystander); err != nil {
		t.Fatalf("备份清理删除了非本次创建的临时目录: %v", err)
	}
}

func TestWorldBackupCopyFailureLeavesSourceUnchanged(t *testing.T) {
	store, source, destination := newWorldBackupFixture(t)
	blocked := filepath.Join(source, "players", "blocked.player")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("写入受限文件失败: %v", err)
	}
	before := snapshotWorldBackupSource(t, source)
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatalf("限制源文件权限失败: %v", err)
	}

	err := store.Backup(context.Background(), destination)
	if restoreErr := os.Chmod(blocked, 0o600); restoreErr != nil {
		t.Fatalf("恢复源文件权限失败: %v", restoreErr)
	}
	if err == nil {
		t.Fatal("复制不可读文件时 Backup() 未返回错误")
	}

	after := snapshotWorldBackupSource(t, source)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("复制失败改变了源世界\nBefore: %#v\nAfter:  %#v", before, after)
	}
	assertWorldBackupAbsent(t, destination)
}

func newWorldBackupFixture(t *testing.T) (*DiskStore, string, string) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "world")
	destination := filepath.Join(base, "backup")
	store, err := OpenDisk(context.Background(), source, OpenOptions{
		Create: Metadata{FormatVersion: currentMetadataVersion, Seed: 42},
	})
	if err != nil {
		t.Fatalf("打开磁盘世界失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("关闭磁盘世界失败: %v", err)
		}
	})
	key := core.ChunkKey{Dimension: 0, Pos: core.ChunkPos{X: -33, Z: 34}}
	if _, err := store.SaveBatch(context.Background(), diskSavesFor([]core.ChunkKey{key}, 1)); err != nil {
		t.Fatalf("保存区块夹具失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "players", "player-one.player"), []byte("player-data"), 0o600); err != nil {
		t.Fatalf("写入玩家夹具失败: %v", err)
	}
	return store, source, destination
}

func assertSameFileContents(t *testing.T, source, destination string) {
	t.Helper()
	want, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("读取源文件 %q 失败: %v", source, err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("读取备份文件 %q 失败: %v", destination, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("备份文件 %q 内容 = %q，期望 %q", destination, got, want)
	}
}

func readWorldBackupIdentity(t *testing.T, destination string) backupIdentity {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(destination, ".mcgo-world-backup-v1.json"))
	if err != nil {
		t.Fatalf("读取备份身份失败: %v", err)
	}
	var identity backupIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatalf("解析备份身份失败: %v", err)
	}
	return identity
}

type worldBackupSnapshot struct {
	Mode fs.FileMode
	Data string
}

func snapshotWorldBackupSource(t *testing.T, root string) map[string]worldBackupSnapshot {
	t.Helper()
	snapshot := make(map[string]worldBackupSnapshot)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := worldBackupSnapshot{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Data = string(data)
		}
		snapshot[relativePath] = item
		return nil
	})
	if err != nil {
		t.Fatalf("快照源世界失败: %v", err)
	}
	return snapshot
}

// assertBackupFreeOfTemporaryResidue 断言备份目录内没有任何原子替换、Compact
// 或 CreateRegion 的临时文件残留，正式文件（含身份文件）不受影响。
func assertBackupFreeOfTemporaryResidue(t *testing.T, destination string) {
	t.Helper()
	err := filepath.WalkDir(destination, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		matchedTemporary, err := filepath.Match(".*.tmp-*", entry.Name())
		if err != nil {
			return err
		}
		matchedCompact, err := filepath.Match(".*.compact-*", entry.Name())
		if err != nil {
			return err
		}
		matchedCreate, err := filepath.Match(".*.create-*", entry.Name())
		if err != nil {
			return err
		}
		if matchedTemporary || matchedCompact || matchedCreate {
			t.Errorf("备份包含临时文件残留 %q", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描备份临时残留失败: %v", err)
	}
}

func assertWorldBackupAbsent(t *testing.T, destination string) {
	t.Helper()
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("失败后正式备份目标不应存在，Lstat 错误: %v", err)
	}
}
