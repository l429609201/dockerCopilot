package containerops

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTerminalDirectoryRealProbe(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd\n\n")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		perm os.FileMode
	}{{"normal file", 0600}, {".a", 0600}, {"line\nbreak", 0600}, {"execute", 0700}} {
		if err := os.WriteFile(filepath.Join(cwd, item.name), []byte("not read by probe"), item.perm); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(cwd, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("subdir", filepath.Join(cwd, "dir-link")); err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(nonce[:])
	shell := exec.Command("/bin/sh", "-c", "export DC_TERMINAL_PID=$$; exec /bin/sh -c 'cd \"$1\"; exec sleep 20' sh \"$1\"", "sh", cwd)
	shell.Env = append(os.Environ(), "DC_TERMINAL_TOKEN="+token)
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shell.Process.Kill(); _ = shell.Wait() }()
	var output []byte
	var err error
	for i := 0; i < 20; i++ {
		output, err = exec.Command("/bin/sh", "-c", directoryProbe, "probe", token).Output()
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("real directory probe: %v", err)
	}
	got, err := parseTerminalDirectory(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != cwd || got.Truncated {
		t.Fatalf("cwd=%q want=%q truncated=%t", got.Path, cwd, got.Truncated)
	}
	entries := make(map[string]string)
	for _, e := range got.Entries {
		entries[e.Name] = e.Kind
	}
	for name, kind := range map[string]string{"normal file": "file", ".a": "file", "line\nbreak": "file", "execute": "executable", "subdir": "directory", "dir-link": "symlink"} {
		if entries[name] != kind {
			t.Fatalf("%q: kind=%q want=%q all=%v", name, entries[name], kind, entries)
		}
	}
	if len(entries) != 6 {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestTerminalDirectoryInvalidProtocol(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("P\x00/tmp\x00"), []byte("P\x00/tmp\x00E\x00file\x00name\x00"), []byte("P\x00/tmp\x00E\x00hacked\x00name\x00T\x000\x00"), []byte("P\x00/tmp\x00E\x00file\x00../bad\x00T\x000\x00")} {
		if _, err := parseTerminalDirectory(data); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}
