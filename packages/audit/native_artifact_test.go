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
			f.replaceFileSize(t, f.paths[0], "invalid")
		}},
		{"size mismatch", func(t *testing.T, f *nativeArtifactFixture) {
			f.replaceFileSize(t, f.paths[0], "999")
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
		{"NUL byte", func(t *testing.T, f *nativeArtifactFixture) { f.insertManifestNUL(t) }},
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

func TestNativeArtifactPackagerRejectsUnsafePublicationInputs(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, fixture *nativeArtifactFixture)
	}{
		{
			name: "newline artifact argument",
			setup: func(t *testing.T, f *nativeArtifactFixture) {
				output := f.packageWith(t, []string{
					"bin/libmornlea_engine.so\nbin/mornlea-server",
					"packages/engine/target/release/libmornlea_engine.so",
				}, nil, false)
				if !strings.Contains(output, "invalid repository-relative path") {
					t.Fatalf("newline argument output = %q", output)
				}
			},
		},
		{
			name: "symlink alias overlaps artifact",
			setup: func(t *testing.T, f *nativeArtifactFixture) {
				if err := os.Symlink(f.root, filepath.Join(f.root, "alias")); err != nil {
					t.Fatal(err)
				}
				f.manifest = "alias/bin/libmornlea_engine.so"
				original := readFile(t, filepath.Join(f.root, f.paths[0]))
				output := f.packageWith(t, f.paths, nil, false)
				if !strings.Contains(output, "symlink") {
					t.Fatalf("symlink alias output = %q", output)
				}
				if got := readFile(t, filepath.Join(f.root, f.paths[0])); string(got) != string(original) {
					t.Fatal("symlink alias overwrote the artifact")
				}
			},
		},
		{
			name: "manifest destination is directory",
			setup: func(t *testing.T, f *nativeArtifactFixture) {
				f.manifest = "manifest-destination"
				if err := os.Mkdir(filepath.Join(f.root, f.manifest), 0o755); err != nil {
					t.Fatal(err)
				}
				output := f.packageWith(t, f.paths, nil, false)
				if !strings.Contains(output, "manifest destination") {
					t.Fatalf("directory destination output = %q", output)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeArtifactFixture(t, "linux-amd64")
			test.setup(t, fixture)
			if _, err := os.Stat(filepath.Join(fixture.root, fixture.manifest)); !os.IsNotExist(err) && test.name == "newline artifact argument" {
				t.Fatalf("unsafe package request wrote a manifest: %v", err)
			}
		})
	}
}

func TestNativeArtifactCommandFailuresStopPublication(t *testing.T) {
	for _, test := range []struct {
		name    string
		script  string
		command string
		verify  bool
	}{
		{"sort", "package-native-artifact.sh", "sort", false},
		{"packager hash", "package-native-artifact.sh", "shasum", false},
		{"verifier hash", "verify-native-artifact.sh", "shasum", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeArtifactFixture(t, "linux-amd64")
			if test.verify {
				fixture.packageManifest(t)
			}
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, test.command), "#!/usr/bin/env bash\nexit 42\n")
			environment := []string{"PATH=" + bin + ":" + os.Getenv("PATH")}
			var output string
			if test.verify {
				output = fixture.verifyWith(t, environment, false)
			} else {
				output = fixture.packageWith(t, fixture.paths, environment, false)
			}
			if !strings.Contains(output, "cannot") {
				t.Fatalf("%s failure output = %q", test.script, output)
			}
			if _, err := os.Stat(filepath.Join(fixture.root, "packages/engine/target/release/deps")); !os.IsNotExist(err) {
				t.Fatalf("%s failure published deps: %v", test.script, err)
			}
			if !test.verify {
				if _, err := os.Stat(filepath.Join(fixture.root, fixture.manifest)); !os.IsNotExist(err) {
					t.Fatalf("%s failure published manifest: %v", test.script, err)
				}
			}
		})
	}
}

func TestNativeArtifactVerifierRejectsUnsafeDestinations(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, fixture *nativeArtifactFixture, outside string)
	}{
		{
			name: "deps symlink",
			setup: func(t *testing.T, f *nativeArtifactFixture, outside string) {
				deps := filepath.Join(f.root, "packages/engine/target/release/deps")
				if err := os.Symlink(outside, deps); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "destination file symlink",
			setup: func(t *testing.T, f *nativeArtifactFixture, outside string) {
				deps := filepath.Join(f.root, "packages/engine/target/release/deps")
				if err := os.Mkdir(deps, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "libmornlea_engine.so"), filepath.Join(deps, "libmornlea_engine.so")); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNativeArtifactFixture(t, "linux-amd64")
			fixture.packageManifest(t)
			outside := t.TempDir()
			outsideLibrary := filepath.Join(outside, "libmornlea_engine.so")
			writeFile(t, outsideLibrary, []byte("outside must remain unchanged\n"))
			test.setup(t, fixture, outside)
			output := fixture.verifyWith(t, nil, false)
			if !strings.Contains(output, "unsafe publication") && !strings.Contains(output, "symlink") {
				t.Fatalf("unsafe destination output = %q", output)
			}
			if got := string(readFile(t, outsideLibrary)); got != "outside must remain unchanged\n" {
				t.Fatalf("verification wrote outside root: %q", got)
			}
		})
	}
}

func TestNativeArtifactVerifierRejectsNULBeforeParsing(t *testing.T) {
	fixture := newNativeArtifactFixture(t, "linux-amd64")
	fixture.packageManifest(t)
	fixture.insertManifestNUL(t)
	output := fixture.verifyWith(t, nil, false)
	if !strings.Contains(output, "manifest contains NUL byte") {
		t.Fatalf("NUL manifest output = %q", output)
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
	f.packageWith(t, f.paths, nil, true)
}

func (f *nativeArtifactFixture) packageWith(t *testing.T, paths, environment []string, wantSuccess bool) string {
	t.Helper()
	arguments := []string{"--platform", f.platform, "--sha", nativeArtifactSHA, "--root", f.root, "--manifest", f.manifest, "--"}
	for index := len(paths) - 1; index >= 0; index-- {
		arguments = append(arguments, paths[index])
	}
	return f.runWith(t, "package-native-artifact.sh", arguments, environment, wantSuccess)
}

func (f *nativeArtifactFixture) verify(t *testing.T, wantSuccess bool) {
	t.Helper()
	f.run(t, "verify-native-artifact.sh", []string{"--platform", f.platform, "--sha", nativeArtifactSHA, "--root", f.root, "--manifest", f.manifest}, wantSuccess)
}

func (f *nativeArtifactFixture) verifyWith(t *testing.T, environment []string, wantSuccess bool) string {
	t.Helper()
	return f.runWith(t, "verify-native-artifact.sh", []string{"--platform", f.platform, "--sha", nativeArtifactSHA, "--root", f.root, "--manifest", f.manifest}, environment, wantSuccess)
}

func (f *nativeArtifactFixture) run(t *testing.T, script string, arguments []string, wantSuccess bool) {
	t.Helper()
	f.runWith(t, script, arguments, nil, wantSuccess)
}

func (f *nativeArtifactFixture) runWith(t *testing.T, script string, arguments, environment []string, wantSuccess bool) string {
	t.Helper()
	command := exec.Command(filepath.Join(repositoryRoot(t), "scripts", "ci", script), arguments...)
	command.Dir = repositoryRoot(t)
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("%s failed: %v\n%s", script, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("%s unexpectedly succeeded:\n%s", script, output)
	}
	return string(output)
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

func (f *nativeArtifactFixture) replaceFileSize(t *testing.T, path, size string) {
	t.Helper()
	old := "file " + path + " " + f.sizeAndDigest(t, path)
	replacement := "file " + path + " " + size + " " + f.digest(t, path)
	f.replaceManifest(t, old, replacement)
}

func (f *nativeArtifactFixture) insertManifestNUL(t *testing.T) {
	t.Helper()
	path := filepath.Join(f.root, f.manifest)
	contents := readFile(t, path)
	needle := []byte("version 1")
	index := strings.Index(string(contents), string(needle))
	if index < 0 {
		t.Fatal("manifest version record is missing")
	}
	mutated := append([]byte(nil), contents[:index+len("version")]...)
	mutated = append(mutated, 0)
	mutated = append(mutated, contents[index+len("version"):]...)
	writeFile(t, path, mutated)
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

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	writeFile(t, path, []byte(contents))
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
