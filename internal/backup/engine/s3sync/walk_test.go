package s3sync

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestObjectKey(t *testing.T) {
	root := "/home/user/myfolder"
	key, err := objectKey(root, filepath.Join(root, "sub", "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "myfolder/sub/file.txt"; key != want {
		t.Errorf("objectKey = %q, want %q", key, want)
	}
}

func TestWalkLocal(t *testing.T) {
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "top.txt"), "hello")
	mustWriteFile(t, filepath.Join(root, "nested", "deep.txt"), "world")
	mustWriteFile(t, filepath.Join(root, "empty.txt"), "")
	if err := os.MkdirAll(filepath.Join(root, "empty-dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	linkTarget := filepath.Join(root, "top.txt")
	linkPath := filepath.Join(root, "link.txt")
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}

	files, err := walkLocal(root)
	if err != nil {
		t.Fatalf("walkLocal: %v", err)
	}

	base := filepath.Base(root)
	var gotKeys []string
	bySize := map[string]int64{}
	for _, f := range files {
		gotKeys = append(gotKeys, f.Key)
		bySize[f.Key] = f.Size
	}
	sort.Strings(gotKeys)

	wantKeys := []string{
		base + "/empty.txt",
		base + "/nested/deep.txt",
		base + "/top.txt",
	}
	sort.Strings(wantKeys)

	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("got keys %v, want %v (symlink/empty-dir must be excluded)", gotKeys, wantKeys)
	}
	for i := range gotKeys {
		if gotKeys[i] != wantKeys[i] {
			t.Errorf("got keys %v, want %v", gotKeys, wantKeys)
			break
		}
	}

	if bySize[base+"/empty.txt"] != 0 {
		t.Errorf("empty.txt size = %d, want 0", bySize[base+"/empty.txt"])
	}
	if bySize[base+"/top.txt"] != int64(len("hello")) {
		t.Errorf("top.txt size = %d, want %d", bySize[base+"/top.txt"], len("hello"))
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
