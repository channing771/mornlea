package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPatchSettingsAtomicCommitPointFaults(t *testing.T) {
	wantErr := errors.New("injected persistence failure")
	for _, test := range []struct {
		name          string
		stage         string
		wantCommitted bool
		wantEvents    []string
	}{
		{name: "directory open", stage: "open", wantEvents: []string{"open"}},
		{name: "rename", stage: "rename", wantEvents: []string{"open", "rename", "close"}},
		{name: "directory sync", stage: "sync", wantCommitted: true, wantEvents: []string{"open", "rename", "sync", "close"}},
		{name: "directory close", stage: "close", wantCommitted: true, wantEvents: []string{"open", "rename", "sync", "close"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			before := []byte(`{"version":1,"audioVolume":0.7,"windowSize":"1280x720","future":{"keep":true}}`)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			var events []string
			ops := defaultAtomicFileOps()
			openDirectory := ops.openDirectory
			ops.openDirectory = func(path string) (syncDirectory, error) {
				events = append(events, "open")
				if test.stage == "open" {
					return nil, wantErr
				}
				directory, err := openDirectory(path)
				if err != nil {
					return nil, err
				}
				return &faultDirectory{
					inner:  directory,
					events: &events,
					syncErr: func() error {
						if test.stage == "sync" {
							return wantErr
						}
						return nil
					}(),
					closeErr: func() error {
						if test.stage == "close" {
							return wantErr
						}
						return nil
					}(),
				}, nil
			}
			rename := ops.rename
			ops.rename = func(oldPath, newPath string) error {
				events = append(events, "rename")
				if test.stage == "rename" {
					return wantErr
				}
				return rename(oldPath, newPath)
			}

			result, err := patchSettingsWithFileOps(path, SettingsPatch{
				AudioVolume: 0.25, TexturePackPath: "packs/next", WindowSize: WindowSize960x540,
			}, ops)
			if err == nil {
				t.Fatal("故障注入未返回错误")
			}
			if result.Committed != test.wantCommitted {
				t.Fatalf("Committed=%v want=%v err=%v", result.Committed, test.wantCommitted, err)
			}
			var persistenceError *PersistenceError
			if !errors.As(err, &persistenceError) || persistenceError.Committed() != test.wantCommitted {
				t.Fatalf("错误未携带提交点: %T %v", err, err)
			}
			if !reflect.DeepEqual(events, test.wantEvents) {
				t.Fatalf("I/O 顺序=%v want=%v", events, test.wantEvents)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !test.wantCommitted {
				if !bytes.Equal(after, before) {
					t.Fatalf("rename 前失败改写目标: before=%q after=%q", before, after)
				}
			} else {
				loaded, loadErr := Load(path)
				if loadErr != nil {
					t.Fatalf("已提交目标不可重载: %v", loadErr)
				}
				if loaded.AudioVolume != 0.25 || loaded.TexturePackPath != "packs/next" || loaded.WindowSize != WindowSize960x540 {
					t.Fatalf("已提交目标未包含 patch: %+v", loaded)
				}
			}
			temps, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), "config-*.json.tmp"))
			if globErr != nil || len(temps) != 0 {
				t.Fatalf("临时文件未清理: %v err=%v", temps, globErr)
			}
		})
	}
}

func TestConfigSaveReportsPostCommitDurabilityFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	ops := defaultAtomicFileOps()
	openDirectory := ops.openDirectory
	wantErr := errors.New("directory sync failed")
	ops.openDirectory = func(path string) (syncDirectory, error) {
		directory, err := openDirectory(path)
		if err != nil {
			return nil, err
		}
		return &faultDirectory{inner: directory, syncErr: wantErr}, nil
	}

	result, err := Defaults().saveWithFileOps(path, ops)
	if err == nil || !result.Committed {
		t.Fatalf("Save result=%+v err=%v", result, err)
	}
	var persistenceError *PersistenceError
	if !errors.As(err, &persistenceError) || !persistenceError.Committed() {
		t.Fatalf("Save 错误未标记已提交: %T %v", err, err)
	}
	if _, loadErr := Load(path); loadErr != nil {
		t.Fatalf("已提交配置不可重载: %v", loadErr)
	}
}

type faultDirectory struct {
	inner    syncDirectory
	events   *[]string
	syncErr  error
	closeErr error
}

func (directory *faultDirectory) Sync() error {
	if directory.events != nil {
		*directory.events = append(*directory.events, "sync")
	}
	if directory.syncErr != nil {
		return directory.syncErr
	}
	return directory.inner.Sync()
}

func (directory *faultDirectory) Close() error {
	if directory.events != nil {
		*directory.events = append(*directory.events, "close")
	}
	err := directory.inner.Close()
	return errors.Join(err, directory.closeErr)
}
