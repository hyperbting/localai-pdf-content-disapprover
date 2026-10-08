package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestHelperProcess is not a real test: it is the program the exec provider
// runs. It approves, and writes the arguments it got after "--" to
// HELPER_ARGS_FILE so the test can check them.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	io.Copy(io.Discard, os.Stdin)
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) > 0 {
		args = args[1:]
	}
	if err := os.WriteFile(os.Getenv("HELPER_ARGS_FILE"), []byte(strings.Join(args, "\n")), 0o644); err != nil {
		os.Exit(1)
	}
	fmt.Print(`{"verdict":"approve","findings":[]}`)
	os.Exit(0)
}

func TestExecArgKeepsSpaces(t *testing.T) {
	// Copy the test binary into a folder whose name has a space, then run it
	// through --exec with arguments that contain spaces too.
	dir := filepath.Join(t.TempDir(), "my models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	prog := filepath.Join(dir, name)
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prog, data, 0o755); err != nil {
		t.Fatal(err)
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	t.Setenv("HELPER_ARGS_FILE", argsFile)
	pdf := writePDF(t, t.TempDir(), "Quarterly results are good")

	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"review", pdf, "-q", "-p", "exec", "--exec", prog,
		"--exec-arg", "-test.run=^TestHelperProcess$", "--exec-arg", "--",
		"--exec-arg", "--model", "--exec-arg", `D:\my models\q 4.gguf`})
	if err := root.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.HasPrefix(out.String(), "APPROVED") {
		t.Fatalf("output: %s", out.String())
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "--model\nD:\\my models\\q 4.gguf"; string(got) != want {
		t.Errorf("program got args %q, want %q", got, want)
	}
}

func TestExecCommand(t *testing.T) {
	file := filepath.Join(t.TempDir(), "run model.cmd")
	os.WriteFile(file, nil, 0o755)

	cases := []struct {
		exec string
		args []string
		want []string
	}{
		{"python my_model.py", nil, []string{"python", "my_model.py"}},
		{file, nil, []string{file}},
		{`C:\Program Files\x\run.exe`, []string{"--model", `D:\a b\q.gguf`}, []string{`C:\Program Files\x\run.exe`, "--model", `D:\a b\q.gguf`}},
		{"", nil, nil},
	}
	for _, c := range cases {
		a := &app{execCmd: c.exec, execArgs: c.args}
		if got := a.command(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("exec %q args %q: got %q, want %q", c.exec, c.args, got, c.want)
		}
	}
}

func TestExecArgNeedsExec(t *testing.T) {
	pdf := writePDF(t, t.TempDir(), "x")
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"review", pdf, "-p", "exec", "--exec-arg", "x"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--exec-arg needs --exec") {
		t.Fatalf("err = %v", err)
	}
}
