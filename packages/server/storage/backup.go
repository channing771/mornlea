package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	backupIdentityName     = ".mcgo-world-backup-v1.json"
	backupMigrationVersion = 1
	maxBackupIdentitySize  = int64(4 << 10)
)

type backupIdentity struct {
	Source           string `json:"source"`
	Seed             int64  `json:"seed"`
	MigrationVersion int    `json:"migration_version"`
}

type backupDirectory struct {
	path string
	mode fs.FileMode
}

// Backup 在当前世界锁内创建可验证的完整目录备份。region 文件的复制经与
// 保存方相同的缓存句柄在 per-region 读锁下与同 region 的 Save/Compact 写
// 提交串行：复制的 header 与数据区同属一个已提交代，双 bank 修复语义在
// 备份副本内保持成立。非 region 文件（world.meta、players 等原子替换聚合）
// 为尽力复制；复制全程不长时间持有 regionMu 或任何类别锁，不同 region 与
// 不同类别的并行保存不受阻塞。
func (store *DiskStore) Backup(ctx context.Context, destination string) error {
	return store.backup(ctx, destination, os.Rename, syncDirectory)
}

func (store *DiskStore) backup(
	ctx context.Context,
	destination string,
	rename func(string, string) error,
	syncDir func(string) error,
) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.metadataMu.Lock()
	seed := store.files.metadata.Seed
	closed := store.closed.Load()
	store.metadataMu.Unlock()
	if closed {
		return os.ErrClosed
	}

	source, err := filepath.Abs(store.files.root)
	if err != nil {
		return fmt.Errorf("resolve world path %q: %w", store.files.root, err)
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve backup path %q: %w", destination, err)
	}
	comparisonSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve world path aliases %q: %w", source, err)
	}
	comparisonParent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("resolve backup parent aliases %q: %w", destination, err)
	}
	comparisonDestination := filepath.Join(comparisonParent, filepath.Base(destination))
	inside, err := pathInside(comparisonSource, comparisonDestination)
	if err != nil {
		return fmt.Errorf("compare world and backup paths: %w", err)
	}
	if inside {
		return fmt.Errorf("backup path %q is inside world %q", destination, source)
	}

	identity := backupIdentity{
		Source:           source,
		Seed:             seed,
		MigrationVersion: backupMigrationVersion,
	}
	if exists, err := matchingBackup(destination, identity); err != nil {
		return err
	} else if exists {
		if err := syncDir(filepath.Dir(destination)); err != nil {
			return fmt.Errorf("sync existing backup parent for %q: %w", destination, err)
		}
		return nil
	}

	temporary, err := os.MkdirTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary backup beside %q: %w", destination, err)
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.RemoveAll(temporary)
		}
	}()

	directories, err := store.copyWorldBackup(ctx, source, temporary)
	if err != nil {
		return err
	}
	identityData, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("encode backup identity: %w", err)
	}
	identityData = append(identityData, '\n')
	if err := writeBackupFile(filepath.Join(temporary, backupIdentityName), identityData, 0o600); err != nil {
		return fmt.Errorf("write backup identity: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		directory := directories[index]
		if err := os.Chmod(directory.path, directory.mode.Perm()); err != nil {
			return fmt.Errorf("set backup directory mode %q: %w", directory.path, err)
		}
		if err := syncDir(directory.path); err != nil {
			return fmt.Errorf("sync backup directory %q: %w", directory.path, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("backup destination %q appeared while copying", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination %q: %w", destination, err)
	}
	if err := rename(temporary, destination); err != nil {
		return fmt.Errorf("publish backup %q: %w", destination, err)
	}
	removeTemporary = false
	if err := syncDir(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("sync backup parent for %q: %w", destination, err)
	}
	return nil
}

func pathInside(parent, candidate string) (bool, error) {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false, err
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

func matchingBackup(destination string, want backupIdentity) (bool, error) {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect backup destination %q: %w", destination, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("backup destination %q is not a real directory", destination)
	}

	identityPath := filepath.Join(destination, backupIdentityName)
	identityInfo, err := os.Lstat(identityPath)
	if err != nil {
		return false, fmt.Errorf("inspect backup identity %q: %w", identityPath, err)
	}
	if !identityInfo.Mode().IsRegular() {
		return false, fmt.Errorf("backup identity %q is not a regular file", identityPath)
	}
	if identityInfo.Size() > maxBackupIdentitySize {
		return false, fmt.Errorf("backup identity %q exceeds %d bytes", identityPath, maxBackupIdentitySize)
	}
	file, err := os.Open(identityPath)
	if err != nil {
		return false, fmt.Errorf("open backup identity %q: %w", identityPath, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxBackupIdentitySize+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return false, fmt.Errorf("read backup identity %q: %w", identityPath, err)
	}
	if int64(len(data)) > maxBackupIdentitySize {
		return false, fmt.Errorf("backup identity %q exceeds %d bytes", identityPath, maxBackupIdentitySize)
	}
	var got backupIdentity
	if err := json.Unmarshal(data, &got); err != nil {
		return false, fmt.Errorf("decode backup identity %q: %w", identityPath, err)
	}
	if got != want {
		return false, fmt.Errorf("backup destination %q belongs to another world or migration", destination)
	}
	return true, nil
}

// copyWorldBackup 遍历 source 世界目录把全部正式条目复制到 destination 下
// 的新树：region 布局文件经缓存句柄读锁复制（与保存提交串行），其余文件
// 直读复制；全部临时命名家族与 world.lock 被跳过。返回已创建目录清单供
// 调用方收尾同步。
func (store *DiskStore) copyWorldBackup(ctx context.Context, source, destination string) ([]backupDirectory, error) {
	directories := make([]backupDirectory, 0, 8)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("world entry %q is a symlink", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "world.lock" || relative == backupIdentityName {
			return nil
		}
		// 跳过原子替换与 region 落盘的全部临时命名家族：聚合文件的 `.tmp-*`、
		// Compact 的 `.compact-*` 与 CreateRegion 的 `.create-*`，半写候选
		// 不得混入备份；备份自身的临时目录位于目标侧，不在此处理。
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
		if relative != "." && (matchedTemporary || matchedCompact || matchedCreate) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			target = filepath.Join(destination, relative)
		}
		if info.IsDir() {
			if relative != "." {
				if err := os.Mkdir(target, 0o700); err != nil {
					return err
				}
			}
			directories = append(directories, backupDirectory{path: target, mode: info.Mode()})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("world entry %q is not a regular file or directory", path)
		}
		if regionKey, ok := regionKeyForBackupPath(relative); ok {
			if err := store.copyRegionBackup(ctx, regionKey, path, target, info.Mode()); err != nil {
				return err
			}
			return nil
		}
		if err := copyBackupFile(ctx, path, target, info.Mode()); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("copy world %q: %w", source, err)
	}
	return directories, nil
}

func copyBackupFile(ctx context.Context, source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", source, err)
	}
	copyErr := copyBackupIntoNewFile(source, destination, mode, func(writer io.Writer) error {
		_, err := io.Copy(writer, contextReader{ctx: ctx, reader: input})
		return err
	})
	return errors.Join(copyErr, input.Close())
}

// regionKeyForBackupPath 判定世界根内的相对路径是否为 DiskStore 布局的
// region 文件 `dimensions/<dim>/regions/r.<X>.<Z>.region` 并解析出缓存键；
// 数字分量一律要求规范十进制形式，`+0`、`08` 之类旁路命名不进缓存路径，
// 按普通文件尽力复制。
func regionKeyForBackupPath(relative string) (RegionKey, bool) {
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || parts[0] != "dimensions" || parts[2] != "regions" {
		return RegionKey{}, false
	}
	dimension, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil || parts[1] != strconv.FormatInt(dimension, 10) {
		return RegionKey{}, false
	}
	name, ok := strings.CutPrefix(parts[3], "r.")
	if !ok {
		return RegionKey{}, false
	}
	name, ok = strings.CutSuffix(name, ".region")
	if !ok {
		return RegionKey{}, false
	}
	coordinates := strings.Split(name, ".")
	if len(coordinates) != 2 {
		return RegionKey{}, false
	}
	regionX, errX := strconv.ParseInt(coordinates[0], 10, 32)
	regionZ, errZ := strconv.ParseInt(coordinates[1], 10, 32)
	if errX != nil || errZ != nil ||
		coordinates[0] != strconv.FormatInt(regionX, 10) ||
		coordinates[1] != strconv.FormatInt(regionZ, 10) {
		return RegionKey{}, false
	}
	return RegionKey{
		Dimension: core.DimensionID(dimension),
		X:         int32(regionX),
		Z:         int32(regionZ),
	}, true
}

// copyRegionBackup 经与保存方相同的缓存路径复制单个 region 文件：句柄在
// regionMu 内取用并登记在途引用（LRU 淘汰与 Close 排空都不会关闭复制中的
// 句柄），复制本体在容器读锁内进行，与同 region 的 Save/Compact 写提交
// 互斥，备份字节因此总对应某个完整提交点。store 已进入排空（Close）时新
// 的句柄取用失败，备份以 os.ErrClosed 中止；在途复制先行排空后 Close 才
// 关闭句柄，两者不交叠。
func (store *DiskStore) copyRegionBackup(
	ctx context.Context,
	key RegionKey,
	source, destination string,
	mode fs.FileMode,
) error {
	handle, err := store.acquireRegion(ctx, key, false)
	if err != nil {
		return fmt.Errorf("acquire region %+v for backup: %w", key, err)
	}
	defer store.releaseRegion(handle)
	return copyBackupIntoNewFile(source, destination, mode, func(writer io.Writer) error {
		_, err := handle.CopyTo(ctx, writer)
		return err
	})
}

// copyBackupIntoNewFile 以排他方式创建 destination，把 copy 回调写入的内容
// 落盘并完成 fsync、关闭与权限对齐；source 仅用于错误定位。直读文件与
// 读锁内经缓存句柄读取两条备份路径共用此落盘骨架。
func copyBackupIntoNewFile(
	source, destination string,
	mode fs.FileMode,
	copy func(io.Writer) error,
) error {
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return fmt.Errorf("create backup file %q: %w", destination, err)
	}
	copyErr := copy(output)
	var syncErr error
	if copyErr == nil {
		syncErr = output.Sync()
	}
	closeErr := output.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("copy %q to %q: %w", source, destination, err)
	}
	if err := os.Chmod(destination, mode.Perm()); err != nil {
		return fmt.Errorf("set backup file mode %q: %w", destination, err)
	}
	return nil
}

func writeBackupFile(path string, data []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	return errors.Join(writeErr, syncErr, file.Close())
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(data)
}
