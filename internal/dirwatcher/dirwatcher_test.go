package dirwatcher

import (
	"os"
	"testing"
	"time"
)

func TestDirWatcher(t *testing.T) {
	testDir := t.TempDir()

	if err := Start(); err != nil {
		t.Errorf("Start failed: %v", err)
	}

	var gotPath string
	var gotOp uint32

	handler := func(path string, op uint32) {
		gotPath = path
		gotOp = op
		t.Logf("path=%s op=%d", gotPath, gotOp)
	}
	if err := WatchDirectory(testDir, handler); err != nil {
		t.Errorf("WatchDirectory failed: %v", err)
	}

	expectedPath := testDir + "/testfile"
	os.WriteFile(expectedPath, []byte{}, 0o644)

	for i := 0; i < 5 && gotPath == ""; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if gotPath != expectedPath {
		t.Errorf("TestDirWatcher expected %s got %s", expectedPath, gotPath)
	}

	Stop()
}
