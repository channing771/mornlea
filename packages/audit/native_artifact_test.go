package archcheck_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const nativeArtifactSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestNativeArtifactPlatformIdentity(t *testing.T) {
	script := filepath.Join(repositoryRoot(t), "scripts", "ci", "platform-id.sh")
	for _, test := range []struct {
		name    string
		system  string
		machine string
		want    string
		fails   bool
	}{
		{"linux amd64", "Linux", "x86_64", "linux-amd64", false},
		{"macos arm64", "Darwin", "arm64", "macos-arm64", false},
		{"macos x86_64", "Darwin", "x86_64", "macos-x86_64", false},
		{"unsupported", "FreeBSD", "amd64", "unsupported CI platform: FreeBSD/amd64", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(script)
			command.Env = append(os.Environ(), "MORNLEA_CI_UNAME_S="+test.system, "MORNLEA_CI_UNAME_M="+test.machine)
			output, err := command.CombinedOutput()
			if test.fails {
				if err == nil {
					t.Fatalf("platform command unexpectedly succeeded: %s", output)
				}
				if string(output) != test.want+"\n" {
					t.Fatalf("unsupported output = %q, want %q", output, test.want+"\n")
				}
				return
			}
			if err != nil {
				t.Fatalf("platform command failed: %v\n%s", err, output)
			}
			if string(output) != test.want+"\n" {
				t.Fatalf("platform output = %q, want %q", output, test.want+"\n")
			}
		})
	}
}

func TestNativeArtifactManifestRoundTrip(t *testing.T) {
	for _, platform := range []string{"linux-amd64", "macos-arm64", "macos-x86_64"} {
		t.Run(platform, func(t *testing.T) {
			fixture := newNativeArtifactFixture(t, platform)
			fixture.packageManifest(t)
			if got := string(readFile(t, filepath.Join(fixture.root, fixture.manifest))); got != fixture.wantManifest(t) {
				t.Fatalf("manifest bytes = %q, want %q", got, fixture.wantManifest(t))
			}
			fixture.verify(t, true)
			for _, library := range fixture.copiedLibraries() {
				got := readFile(t, filepath.Join(fixture.root, "packages/engine/target/release/deps", filepath.Base(library)))
				want := readFile(t, filepath.Join(fixture.root, library))
				if string(got) != string(want) {
					t.Errorf("copied %s bytes differ", library)
				}
			}
		})
	}
}

func TestNativeArtifactManifestMutations(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		mutate func(t *testing.T, fixture *nativeArtifactFixture)
	}{
		{"wrong SHA", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, "sha "+nativeArtifactSHA, "sha "+strings.Repeat("b", 40))
		}},
		{"wrong platform", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, "platform linux-amd64", "platform macos-arm64")
		}},
		{"missing file", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, "file "+f.paths[0]+" "+f.sizeAndDigest(t, f.paths[0])+"\n", "")
		}},
		{"extra file record", func(t *testing.T, f *nativeArtifactFixture) {
			f.appendManifest(t, "file extra-file 1 "+strings.Repeat("a", 64)+"\n")
		}},
		{"duplicate record", func(t *testing.T, f *nativeArtifactFixture) {
			f.appendManifest(t, "file "+f.paths[0]+" "+f.sizeAndDigest(t, f.paths[0])+"\n")
		}},
		{"reversed file order", func(t *testing.T, f *nativeArtifactFixture) { f.reverseFileRecords(t) }},
		{"non decimal size", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, "file "+f.paths[0]+" ", "file "+f.paths[0]+" invalid ")
		}},
		{"size mismatch", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, "file "+f.paths[0]+" ", "file "+f.paths[0]+" 999 ")
		}},
		{"uppercase digest", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, f.digest(t, f.paths[0]), strings.ToUpper(f.digest(t, f.paths[0])))
		}},
		{"incorrect digest", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceManifest(t, f.digest(t, f.paths[0]), strings.Repeat("0", 64))
		}},
		{"absolute path", func(t *testing.T, f *nativeArtifactFixture) { f.replaceManifest(t, f.paths[0], "/tmp/native-artifact") }},
		{"parent traversal", func(t *testing.T, f *nativeArtifactFixture) { f.replaceManifest(t, f.paths[0], "../outside") }},
		{"symlink artifact", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceArtifactWithSymlink(t, f.paths[0], filepath.Join(f.root, f.paths[1]))
		}},
		{"symlink escape", func(t *testing.T, f *nativeArtifactFixture) { f.makeSymlinkEscape(t) }},
		{"trailing fields", func(t *testing.T, f *nativeArtifactFixture) {
			old := "file " + f.paths[0] + " " + f.sizeAndDigest(t, f.paths[0])
			f.replaceManifest(t, old, old+" trailing")
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newNativeArtifactFixture(t, "linux-amd64")
			fixture.packageManifest(t)
			mutation.mutate(t, fixture)
			fixture.verify(t, false)
			if _, err := os.Stat(filepath.Join(fixture.root, "packages/engine/target/release/deps")); !os.IsNotExist(err) {
				t.Fatalf("verification mutation created deps: %v", err)
			}
		})
	}
}

type nativeArtifactFixture struct {
	root     string
	platform string
	paths    []string
	manifest string
}

func newNativeArtifactFixture(t *testing.T, platform string) *nativeArtifactFixture {
	t.Helper()
	fixture := &nativeArtifactFixture{root: t.TempDir(), platform: platform, manifest: "native-artifact-manifest.txt"}
	fixture.paths = nativeArtifactPaths(platform)
	for _, path := range fixture.paths {
		writeFile(t, filepath.Join(fixture.root, path), []byte(platform+":"+path+"\n"))
	}
	return fixture
}

func nativeArtifactPaths(platform string) []string {
	switch platform {
	case "linux-amd64":
		return []string{"bin/libmornlea_engine.so", "bin/mornlea-server", "packages/engine/target/release/libmornlea_engine.so"}
	case "macos-arm64", "macos-x86_64":
		return []string{"packages/engine/target/release/libmornlea_client.dylib", "packages/engine/target/release/libmornlea_engine.dylib"}
	default:
		panic("unsupported fixture platform: " + platform)
	}
}

func (f *nativeArtifactFixture) packageManifest(t *testing.T) {
	t.Helper()
	arguments := []string{"--platform", f.platform, "--sha", nativeArtifactSHA, "--root", f.root, "--manifest", f.manifest, "--"}
	for index := len(f.paths) - 1; index >= 0; index-- {
		arguments = append(arguments, f.paths[index])
	}
	f.run(t, "package-native-artifact.sh", arguments, true)
}

func (f *nativeArtifactFixture) verify(t *testing.T, wantSuccess bool) {
	t.Helper()
	f.run(t, "verify-native-artifact.sh", []string{"--platform", f.platform, "--sha", nativeArtifactSHA, "--root", f.root, "--manifest", f.manifest}, wantSuccess)
}

func (f *nativeArtifactFixture) run(t *testing.T, script string, arguments []string, wantSuccess bool) {
	t.Helper()
	command := exec.Command(filepath.Join(repositoryRoot(t), "scripts", "ci", script), arguments...)
	command.Dir = repositoryRoot(t)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("%s failed: %v\n%s", script, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("%s unexpectedly succeeded:\n%s", script, output)
	}
}

func (f *nativeArtifactFixture) wantManifest(t *testing.T) string {
	t.Helper()
	var builder strings.Builder
	fmt.Fprintf(&builder, "version 1\nsha %s\nplatform %s\n", nativeArtifactSHA, f.platform)
	for _, path := range f.paths {
		fmt.Fprintf(&builder, "file %s %s\n", path, f.sizeAndDigest(t, path))
	}
	return builder.String()
}

func (f *nativeArtifactFixture) sizeAndDigest(t *testing.T, path string) string {
	return fmt.Sprintf("%d %s", len(readFile(t, filepath.Join(f.root, path))), f.digest(t, path))
}

func (f *nativeArtifactFixture) digest(t *testing.T, path string) string {
	return fmt.Sprintf("%x", sha256.Sum256(readFile(t, filepath.Join(f.root, path))))
}

func (f *nativeArtifactFixture) copiedLibraries() []string {
	if f.platform == "linux-amd64" {
		return []string{"packages/engine/target/release/libmornlea_engine.so"}
	}
	return f.paths
}

func (f *nativeArtifactFixture) replaceManifest(t *testing.T, old, replacement string) {
	t.Helper()
	contents := string(readFile(t, filepath.Join(f.root, f.manifest)))
	if !strings.Contains(contents, old) {
		t.Fatalf("manifest does not contain %q", old)
	}
	writeFile(t, filepath.Join(f.root, f.manifest), []byte(strings.Replace(contents, old, replacement, 1)))
}

func (f *nativeArtifactFixture) appendManifest(t *testing.T, record string) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(f.root, f.manifest), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(record); err != nil {
		t.Fatal(err)
	}
}

func (f *nativeArtifactFixture) reverseFileRecords(t *testing.T) {
	t.Helper()
	records := strings.Split(strings.TrimSuffix(string(readFile(t, filepath.Join(f.root, f.manifest))), "\n"), "\n")
	slices.Reverse(records[3:])
	writeFile(t, filepath.Join(f.root, f.manifest), []byte(strings.Join(records, "\n")+"\n"))
}

func (f *nativeArtifactFixture) replaceArtifactWithSymlink(t *testing.T, path, target string) {
	t.Helper()
	fullPath := filepath.Join(f.root, path)
	if err := os.Remove(fullPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, fullPath); err != nil {
		t.Fatal(err)
	}
}

func (f *nativeArtifactFixture) makeSymlinkEscape(t *testing.T) {
	t.Helper()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "libmornlea_engine.so"), []byte("outside\n"))
	bin := filepath.Join(f.root, "bin")
	if err := os.RemoveAll(bin); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, bin); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func writeFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
