package archcheck_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotProjectValidatorRequiresRipgrepBeforeValidation(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	pathRoot := t.TempDir()
	if err := os.Symlink(bashPath, filepath.Join(pathRoot, "bash")); err != nil {
		t.Fatal(err)
	}

	validator := filepath.Join(repositoryRoot(t), "scripts", "godot", "validate-project.sh")
	command := exec.Command(validator)
	command.Env = []string{"PATH=" + pathRoot}
	outputBytes, err := command.CombinedOutput()
	output := string(outputBytes)
	if err == nil {
		t.Fatal("validator accepted an environment without rg")
	}
	if !strings.Contains(output, "missing required executable: rg") {
		t.Fatalf("missing dependency diagnostic:\n%s", output)
	}
	if strings.Contains(output, "validation passed") {
		t.Fatalf("validator reported success after a dependency failure:\n%s", output)
	}
}
