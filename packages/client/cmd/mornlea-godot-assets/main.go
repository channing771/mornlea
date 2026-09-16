// Command mornlea-godot-assets materializes deterministic Godot resources from
// the existing client asset registry and its registered font inputs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/channing771/mornlea/packages/client/assets"
)

const (
	manifestSchemaVersion = 1
	atlasWidth            = 16
	atlasHeight           = 16
	atlasMipLevels        = 5
	manifestName          = "manifest.json"
	fontImportName        = "NotoSansCJKsc-Regular.otf.import"
	fontImportMetadata    = `[remap]

importer="font_data_dynamic"
type="FontFile"
uid="uid://et7oghf10wjg"
path="res://.godot/imported/NotoSansCJKsc-Regular.otf-e98c491611140aed2e70770cc4359f87.fontdata"

[deps]

source_file="res://assets/generated/fonts/NotoSansCJKsc-Regular.otf"
dest_files=["res://.godot/imported/NotoSansCJKsc-Regular.otf-e98c491611140aed2e70770cc4359f87.fontdata"]

[params]

Rendering=null
antialiasing=1
generate_mipmaps=false
disable_embedded_bitmaps=true
multichannel_signed_distance_field=false
msdf_pixel_range=8
msdf_size=48
allow_system_fallback=false
force_autohinter=false
modulate_color_glyphs=false
hinting=3
subpixel_positioning=4
keep_rounding_remainders=true
oversampling=0.0
Fallbacks=null
fallbacks=[]
Compress=null
compress=true
preload=[]
language_support={}
script_support={}
opentype_features={}
`
)

var registeredFontFiles = []string{
	"NotoSansCJKsc-Regular.otf",
	"NotoSansCJKsc-Regular.provenance.json",
	"OFL.txt",
}

var retainedMaterialNotices = []string{
	"ATTRIBUTION.md",
	"LICENSE.txt",
	"PROVENANCE.json",
}

type generationConfig struct {
	repositoryRoot string
	outputRoot     string
	checkOnly      bool
}

type fileRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type revisionRecord struct {
	AssetsGitTree string `json:"assets_git_tree"`
	FontsGitTree  string `json:"fonts_git_tree"`
}

type atlasRecord struct {
	Path         string `json:"path"`
	Format       string `json:"format"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Layers       int    `json:"layers"`
	MipLevels    int    `json:"mip_levels"`
	StorageOrder string `json:"storage_order"`
	SHA256       string `json:"sha256"`
	Bytes        int64  `json:"bytes"`
}

type assetManifest struct {
	SchemaVersion int            `json:"schema_version"`
	Generator     string         `json:"generator"`
	InputRevision revisionRecord `json:"input_revision"`
	InputChecksum string         `json:"input_checksum"`
	Inputs        []fileRecord   `json:"inputs"`
	Atlas         atlasRecord    `json:"atlas"`
	Outputs       []fileRecord   `json:"outputs"`
}

func main() {
	var config generationConfig
	flag.StringVar(&config.repositoryRoot, "repository-root", "", "repository root containing packages/client")
	flag.StringVar(&config.outputRoot, "output", "", "Godot generated asset directory")
	flag.BoolVar(&config.checkOnly, "check", false, "verify generated files without changing them")
	flag.Parse()
	if flag.NArg() != 0 || config.repositoryRoot == "" || config.outputRoot == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(config generationConfig) error {
	repositoryRoot, err := filepath.Abs(config.repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	outputRoot, err := filepath.Abs(config.outputRoot)
	if err != nil {
		return fmt.Errorf("resolve generated asset root: %w", err)
	}
	if err := ensureSafeOutputRoot(repositoryRoot, outputRoot); err != nil {
		return err
	}

	parent := filepath.Dir(outputRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create generated asset parent: %w", err)
	}
	stageRoot, err := os.MkdirTemp(parent, ".generated-stage-")
	if err != nil {
		return fmt.Errorf("create generated asset staging directory: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	if err := materialize(repositoryRoot, stageRoot); err != nil {
		return err
	}
	if config.checkOnly {
		return compareGeneratedTrees(stageRoot, outputRoot)
	}
	if err := rejectUnexpectedGeneratedFiles(stageRoot, outputRoot); err != nil {
		return err
	}
	return replaceGeneratedTree(stageRoot, outputRoot)
}

func ensureSafeOutputRoot(repositoryRoot, outputRoot string) error {
	if outputRoot == repositoryRoot || outputRoot == filepath.VolumeName(outputRoot)+string(filepath.Separator) {
		return fmt.Errorf("generated asset root is too broad: %s", outputRoot)
	}
	if filepath.Base(outputRoot) != "generated" {
		return fmt.Errorf("generated asset root must end in a dedicated generated directory: %s", outputRoot)
	}
	return nil
}

func materialize(repositoryRoot, stageRoot string) error {
	assetsRoot := filepath.Join(repositoryRoot, "packages", "client", "assets")
	fontsRoot := filepath.Join(repositoryRoot, "packages", "client", "render", "assets")
	inputs, err := collectInputRecords(repositoryRoot, assetsRoot, fontsRoot)
	if err != nil {
		return err
	}

	registry := assets.NewDefaultRegistry()
	layerCount, atlasPixels := registry.AtlasPixels()
	atlasPath := filepath.Join(stageRoot, "atlas.rgba8")
	if err := writeFile(atlasPath, atlasPixels); err != nil {
		return err
	}

	for _, name := range registeredFontFiles {
		if err := copyFile(
			filepath.Join(fontsRoot, name),
			filepath.Join(stageRoot, "fonts", name),
		); err != nil {
			return err
		}
	}
	// The import sidecar fixes font identity and import policy across clean
	// workspaces instead of delegating those decisions to an editor cache.
	if err := writeFile(
		filepath.Join(stageRoot, "fonts", fontImportName),
		[]byte(fontImportMetadata),
	); err != nil {
		return err
	}
	materialRoot := filepath.Join(assetsRoot, "packs", "pastelcraft")
	for _, name := range retainedMaterialNotices {
		if err := copyFile(
			filepath.Join(materialRoot, name),
			filepath.Join(stageRoot, "licenses", "pastelcraft", name),
		); err != nil {
			return err
		}
	}

	outputs, err := collectFileRecords(stageRoot, stageRoot)
	if err != nil {
		return err
	}
	atlasOutput, ok := findRecord(outputs, "atlas.rgba8")
	if !ok {
		return errors.New("generated atlas record is missing")
	}
	assetsRevision, err := gitTreeRevision(repositoryRoot, "packages/client/assets")
	if err != nil {
		return err
	}
	fontsRevision, err := gitTreeRevision(repositoryRoot, "packages/client/render/assets")
	if err != nil {
		return err
	}
	manifest := assetManifest{
		SchemaVersion: manifestSchemaVersion,
		Generator:     "packages/client/cmd/mornlea-godot-assets",
		InputRevision: revisionRecord{
			AssetsGitTree: assetsRevision,
			FontsGitTree:  fontsRevision,
		},
		InputChecksum: checksumRecords(inputs),
		Inputs:        inputs,
		Atlas: atlasRecord{
			Path:         atlasOutput.Path,
			Format:       "RGBA8",
			Width:        atlasWidth,
			Height:       atlasHeight,
			Layers:       layerCount,
			MipLevels:    atlasMipLevels,
			StorageOrder: "layer-major,mip-major",
			SHA256:       atlasOutput.SHA256,
			Bytes:        atlasOutput.Bytes,
		},
		Outputs: outputs,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode generated asset manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	return writeFile(filepath.Join(stageRoot, manifestName), encoded)
}

func collectInputRecords(repositoryRoot, assetsRoot, fontsRoot string) ([]fileRecord, error) {
	records, err := collectFileRecords(repositoryRoot, assetsRoot)
	if err != nil {
		return nil, err
	}
	for _, name := range registeredFontFiles {
		record, err := recordFile(repositoryRoot, filepath.Join(fontsRoot, name))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(left, right fileRecord) int {
		return strings.Compare(left.Path, right.Path)
	})
	return records, nil
}

func collectFileRecords(relativeRoot, walkRoot string) ([]fileRecord, error) {
	var records []fileRecord
	err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("asset input or output must not be a symbolic link: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("asset input or output is not a regular file: %s", path)
		}
		record, err := recordFile(relativeRoot, path)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect asset tree %s: %w", walkRoot, err)
	}
	slices.SortFunc(records, func(left, right fileRecord) int {
		return strings.Compare(left.Path, right.Path)
	})
	return records, nil
}

func recordFile(relativeRoot, path string) (fileRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileRecord{}, fmt.Errorf("open asset file %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	bytesWritten, err := io.Copy(hash, file)
	if err != nil {
		return fileRecord{}, fmt.Errorf("hash asset file %s: %w", path, err)
	}
	relative, err := filepath.Rel(relativeRoot, path)
	if err != nil {
		return fileRecord{}, fmt.Errorf("resolve asset path %s: %w", path, err)
	}
	return fileRecord{
		Path:   filepath.ToSlash(relative),
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		Bytes:  bytesWritten,
	}, nil
}

func findRecord(records []fileRecord, path string) (fileRecord, bool) {
	for _, record := range records {
		if record.Path == path {
			return record, true
		}
	}
	return fileRecord{}, false
}

func checksumRecords(records []fileRecord) string {
	hash := sha256.New()
	for _, record := range records {
		hash.Write([]byte(record.Path))
		hash.Write([]byte{0})
		hash.Write([]byte(record.SHA256))
		hash.Write([]byte{0})
		hash.Write([]byte(strconv.FormatInt(record.Bytes, 10)))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func gitTreeRevision(repositoryRoot, path string) (string, error) {
	command := exec.Command("git", "-C", repositoryRoot, "rev-parse", "HEAD:"+path)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Git tree revision for %s: %w", path, err)
	}
	revision := strings.TrimSpace(string(output))
	if len(revision) != 40 && len(revision) != 64 {
		return "", fmt.Errorf("Git tree revision for %s has invalid length: %q", path, revision)
	}
	for _, char := range revision {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return "", fmt.Errorf("Git tree revision for %s is not lowercase hexadecimal: %q", path, revision)
		}
	}
	return revision, nil
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read asset source %s: %w", source, err)
	}
	return writeFile(destination, data)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create asset directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write generated asset %s: %w", path, err)
	}
	return nil
}

func rejectUnexpectedGeneratedFiles(expectedRoot, actualRoot string) error {
	if _, err := os.Lstat(actualRoot); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect generated asset root: %w", err)
	}
	expected, err := collectRelativePaths(expectedRoot)
	if err != nil {
		return err
	}
	actual, err := collectRelativePaths(actualRoot)
	if err != nil {
		return err
	}
	for path := range actual {
		if !expected[path] {
			return fmt.Errorf("unexpected generated asset (hand-authored files are forbidden): %s", path)
		}
	}
	return nil
}

func compareGeneratedTrees(expectedRoot, actualRoot string) error {
	if _, err := os.Lstat(actualRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("generated asset directory is missing: %s", actualRoot)
		}
		return fmt.Errorf("inspect generated asset root: %w", err)
	}
	expected, err := collectRelativePaths(expectedRoot)
	if err != nil {
		return err
	}
	actual, err := collectRelativePaths(actualRoot)
	if err != nil {
		return err
	}
	for path := range actual {
		if !expected[path] {
			return fmt.Errorf("unexpected generated asset (hand-authored files are forbidden): %s", path)
		}
	}
	for path := range expected {
		if !actual[path] {
			return fmt.Errorf("generated asset is missing: %s", path)
		}
		expectedData, err := os.ReadFile(filepath.Join(expectedRoot, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read expected generated asset %s: %w", path, err)
		}
		actualData, err := os.ReadFile(filepath.Join(actualRoot, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read generated asset %s: %w", path, err)
		}
		if !bytes.Equal(expectedData, actualData) {
			return fmt.Errorf("generated asset content differs: %s", path)
		}
	}
	return nil
}

func collectRelativePaths(root string) (map[string]bool, error) {
	paths := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			return fmt.Errorf("unexpected generated asset symbolic link: %s", filepath.ToSlash(relative))
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected generated asset file type: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect generated asset tree %s: %w", root, err)
	}
	return paths, nil
}

func replaceGeneratedTree(stageRoot, outputRoot string) error {
	parent := filepath.Dir(outputRoot)
	backupRoot, err := os.MkdirTemp(parent, ".generated-backup-")
	if err != nil {
		return fmt.Errorf("reserve generated asset backup: %w", err)
	}
	if err := os.Remove(backupRoot); err != nil {
		return fmt.Errorf("prepare generated asset backup: %w", err)
	}
	backupExists := false
	if _, err := os.Lstat(outputRoot); err == nil {
		if err := os.Rename(outputRoot, backupRoot); err != nil {
			return fmt.Errorf("move existing generated assets to backup: %w", err)
		}
		backupExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect existing generated assets: %w", err)
	}

	// The staged directory is on the same filesystem, so this rename exposes a
	// complete generation rather than a partially written asset set.
	if err := os.Rename(stageRoot, outputRoot); err != nil {
		if backupExists {
			_ = os.Rename(backupRoot, outputRoot)
		}
		return fmt.Errorf("install generated asset tree: %w", err)
	}
	if backupExists {
		if err := os.RemoveAll(backupRoot); err != nil {
			return fmt.Errorf("remove generated asset backup %s: %w", backupRoot, err)
		}
	}
	return nil
}
