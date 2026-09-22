package archcheck_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCIDoctorProfilesAndFailures(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "ci", "doctor.sh")
	profiles := map[string][]string{
		"preflight":     {"bash", "git", "go", "gofmt", "node", "npx", "rg"},
		"frontend":      {"bash", "corepack", "git", "node"},
		"rust":          {"bash", "cargo", "rustc", "rustup"},
		"go":            {"bash", "go", "gofmt"},
		"native-linux":  {"bash", "cargo", "cc", "go", "ldd", "make", "nm", "readelf", "rustc", "rustup", "shasum"},
		"native-macos":  {"bash", "cargo", "codesign", "go", "install_name_tool", "make", "nm", "rustc", "rustup", "shasum"},
		"agent":         {"bash", "go", "python3", "uv"},
		"godot-static":  {"bash", "go", "rg", "uv"},
		"godot-runtime": {"bash", "cargo", "nm", "rg", "rustc", "rustup", "uv"},
	}
	for profile, commands := range profiles {
		t.Run(profile, func(t *testing.T) {
			bin := ciFixtureBin(t, commands)
			output, err := ciRun(root, bin, script, profile)
			if err != nil {
				t.Fatalf("doctor %s failed: %v\n%s", profile, err, output)
			}
			if got, want := output, "CI dependency profile passed: "+profile+"\n"; got != want {
				t.Fatalf("doctor output = %q, want %q", got, want)
			}
		})
	}

	t.Run("reports every missing command in lexical order", func(t *testing.T) {
		bin := ciFixtureBin(t, []string{"bash", "git", "go", "gofmt", "node"})
		output, err := ciRun(root, bin, script, "preflight")
		if err == nil {
			t.Fatalf("doctor unexpectedly succeeded: %s", output)
		}
		want := "missing required executable for preflight: npx\nmissing required executable for preflight: rg\n"
		if output != want {
			t.Fatalf("missing output = %q, want %q", output, want)
		}
	})

	t.Run("rejects unknown profile", func(t *testing.T) {
		output, err := ciRun(root, ciFixtureBin(t, []string{"bash"}), script, "unknown")
		if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 2 {
			t.Fatalf("unknown profile error = %v, output = %s", err, output)
		}
		if !strings.Contains(output, "usage: doctor.sh <") {
			t.Fatalf("unknown profile output = %q", output)
		}
	})
}

func TestCIPackagePartitionsRejectMutations(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "ci", "check-package-partitions.sh")
	for _, test := range []struct {
		name   string
		mutate func(*ciPartitionFixture)
		want   string
	}{
		{"valid partition", func(*ciPartitionFixture) {}, ""},
		{"duplicate package", func(f *ciPartitionFixture) { f.write("client", "example/a\nexample/a\n") }, "duplicate package in client: example/a"},
		{"client server overlap", func(f *ciPartitionFixture) { f.write("server", "example/b\nexample/c\n") }, "overlapping package: example/b"},
		{"missing package", func(f *ciPartitionFixture) { f.write("rest", "example/d\n") }, "missing package: example/e"},
		{"unexpected package", func(f *ciPartitionFixture) { f.write("rest", "example/d\nexample/e\nexample/z\n") }, "unexpected package: example/z"},
		{"unsorted package", func(f *ciPartitionFixture) { f.write("client", "example/b\nexample/a\n") }, "package list is not lexically sorted in client: example/a"},
		{"empty slice", func(f *ciPartitionFixture) { f.write("rest", "") }, "package list is empty: rest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCIPartitionFixture(t)
			test.mutate(fixture)
			output, err := ciRun(root, os.Getenv("PATH"), script, fixture.paths()...)
			if test.want == "" {
				if err != nil {
					t.Fatalf("valid partition failed: %v\n%s", err, output)
				}
				return
			}
			if err == nil {
				t.Fatalf("mutation unexpectedly succeeded: %s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("mutation output = %q, want %q", output, test.want)
			}
		})
	}
}

func TestCIRepositoryPackageInventory(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "ci", "package-inventory.sh")
	output, err := ciRun(root, os.Getenv("PATH"), script, "--check")
	if err != nil {
		t.Fatalf("package inventory check failed: %v\n%s", err, output)
	}
	wantModules := []string{"packages/audit", "packages/client", "packages/contracts", "packages/server", "packages/shared", "packages/tools"}
	if got := workspaceModules(t); !slices.Equal(got, wantModules) {
		t.Fatalf("workspace modules = %v, want %v", got, wantModules)
	}
	client := ciPackageInventory(t, root, script, "client")
	rest := ciPackageInventory(t, root, script, "rest")
	gfxspike := "github.com/channing771/mornlea/packages/tools/gfxspike"
	if got := strings.Count(client, gfxspike); got != 1 {
		t.Fatalf("client gfxspike occurrences = %d, want 1: %s", got, client)
	}
	if strings.Contains(rest, gfxspike) {
		t.Fatalf("rest includes Darwin-only gfxspike: %s", rest)
	}
	nativeABI := "github.com/channing771/mornlea/packages/shared/nativeabi"
	if got := strings.Count(rest, nativeABI); got != 1 {
		t.Fatalf("rest nativeabi occurrences = %d, want 1: %s", got, rest)
	}
	inventorySource := string(readFile(t, script))
	if got := strings.Count(inventorySource, "CGO_ENABLED=1"); got < 2 {
		t.Fatalf("inventory must set CGO_ENABLED=1 for both supported platform queries: %q", inventorySource)
	}
	for slice, contents := range map[string]string{"client": client, "server": ciPackageInventory(t, root, script, "server"), "rest": rest} {
		lines := strings.Fields(contents)
		if len(lines) == 0 {
			t.Fatalf("%s package inventory is empty", slice)
		}
		if !slices.IsSorted(lines) {
			t.Fatalf("%s package inventory is not sorted: %v", slice, lines)
		}
	}
}

func TestCIRaceEntrypointArguments(t *testing.T) {
	root := repositoryRoot(t)
	script := filepath.Join(root, "scripts", "ci", "run-go-race.sh")
	bin := t.TempDir()
	argv := filepath.Join(t.TempDir(), "argv")
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "go"), fmt.Sprintf("#!/usr/bin/env bash\nif [[ \"${1:-}\" == test ]]; then\n\tprintf '%%s\\n' \"$@\" > \"$MORNLEA_CI_GO_ARGV\"\n\texit 0\nfi\nexec %q \"$@\"\n", realGo))
	for _, test := range []struct {
		name        string
		slice       string
		wantPackage string
		wantTail    []string
	}{
		{"client skips bounded server probe", "client", "github.com/channing771/mornlea/packages/tools/gfxspike", []string{"-race", "-p=1", "-skip", "^TestScenarioV7EightSessionServerProbeIsRealAndBounded$"}},
		{"server has no client skip", "server", "github.com/channing771/mornlea/packages/server/server", []string{"-race", "-p=1"}},
		{"rest has no client skip", "rest", "github.com/channing771/mornlea/packages/audit", []string{"-race", "-p=1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.Remove(argv); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			output, err := ciRunWithEnv(root, bin+":"+os.Getenv("PATH"), []string{"MORNLEA_CI_GO_ARGV=" + argv}, script, test.slice)
			if err != nil {
				t.Fatalf("race %s failed: %v\n%s", test.slice, err, output)
			}
			got := strings.Fields(string(readFile(t, argv)))
			if len(got) < len(test.wantTail)+2 || got[0] != "test" || !slices.Contains(got, test.wantPackage) || !slices.Equal(got[len(got)-len(test.wantTail):], test.wantTail) {
				t.Fatalf("race argv = %q, want package %q and tail %q", got, test.wantPackage, test.wantTail)
			}
		})
	}
	for _, slice := range []string{"unknown", ""} {
		t.Run(fmt.Sprintf("rejects %q before go test", slice), func(t *testing.T) {
			if err := os.Remove(argv); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			_, err := ciRunWithEnv(root, bin+":"+os.Getenv("PATH"), []string{"MORNLEA_CI_GO_ARGV=" + argv}, script, slice)
			if err == nil {
				t.Fatal("invalid race slice unexpectedly succeeded")
			}
			if _, statErr := os.Stat(argv); !os.IsNotExist(statErr) {
				t.Fatalf("invalid race slice invoked go: %v", statErr)
			}
		})
	}
}

func TestCIPreflightRecipeOrderAndBoundary(t *testing.T) {
	makefile := string(readFile(t, filepath.Join(repositoryRoot(t), "Makefile")))
	recipe := makeTargetRecipe(t, makefile, "ci-preflight")
	ordered := []string{
		"scripts/ci/doctor.sh preflight",
		"gofmt -l",
		"npx --yes @fission-ai/openspec@1.7.0 validate --all --strict --no-interactive",
		"node --test scripts/agent-hooks/guard.test.mjs",
		"$(MAKE) comment-language-check",
		"scripts/ci/package-inventory.sh --check",
		"$(GO) test ./packages/audit -count=1",
	}
	last := -1
	for _, command := range ordered {
		index := strings.Index(recipe, command)
		if index < 0 {
			t.Errorf("ci-preflight is missing %q", command)
			continue
		}
		if index <= last {
			t.Errorf("ci-preflight command order is wrong around %q: %q", command, recipe)
		}
		last = index
	}
	for _, forbidden := range []string{"cargo", "make rust", "package-native-artifact", "verify-native-artifact", "scripts/godot", "godot-"} {
		if strings.Contains(recipe, forbidden) {
			t.Errorf("ci-preflight must not invoke %q: %q", forbidden, recipe)
		}
	}
}

func ciFixtureBin(t *testing.T, commands []string) string {
	t.Helper()
	bin := t.TempDir()
	for _, command := range commands {
		if command == "bash" {
			continue
		}
		writeExecutable(t, filepath.Join(bin, command), "#!/usr/bin/env bash\nexit 0\n")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	return bin + ":" + filepath.Dir(bash)
}

func ciRun(root, path, script string, arguments ...string) (string, error) {
	return ciRunWithEnv(root, path, nil, script, arguments...)
}

func ciRunWithEnv(root, path string, extraEnvironment []string, script string, arguments ...string) (string, error) {
	command := exec.Command(script, arguments...)
	command.Dir = root
	environment := make([]string, 0, len(os.Environ())+len(extraEnvironment)+1)
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "PATH=") {
			environment = append(environment, variable)
		}
	}
	command.Env = append(environment, "PATH="+path)
	command.Env = append(command.Env, extraEnvironment...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func ciPackageInventory(t *testing.T, root, script, slice string) string {
	t.Helper()
	output, err := ciRun(root, os.Getenv("PATH"), script, "--slice", slice)
	if err != nil {
		t.Fatalf("package inventory %s failed: %v\n%s", slice, err, output)
	}
	return output
}

type ciPartitionFixture struct {
	dir string
	t   *testing.T
}

func newCIPartitionFixture(t *testing.T) *ciPartitionFixture {
	t.Helper()
	fixture := &ciPartitionFixture{dir: t.TempDir(), t: t}
	fixture.write("all", "example/a\nexample/b\nexample/c\nexample/d\nexample/e\n")
	fixture.write("client", "example/a\nexample/b\n")
	fixture.write("server", "example/c\n")
	fixture.write("rest", "example/d\nexample/e\n")
	return fixture
}

func (f *ciPartitionFixture) write(name, contents string) {
	writeFile(f.t, filepath.Join(f.dir, name), []byte(contents))
}

func (f *ciPartitionFixture) paths() []string {
	return []string{filepath.Join(f.dir, "all"), filepath.Join(f.dir, "client"), filepath.Join(f.dir, "server"), filepath.Join(f.dir, "rest")}
}
