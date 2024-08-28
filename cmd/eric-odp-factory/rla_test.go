package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func TestRlaOkay(t *testing.T) {
	logDir := t.TempDir()
	logFile := logDir + "/test.log"
	rla, err := NewRollingLogAppender(logFile, 2)
	if err != nil {
		t.Fatalf("Failed to call to NewRollingLogAppender failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		rla.Write([]byte(fmt.Sprintf("write %d\n", i))) //nolint:gocritic // Want to call the Write func
	}

	rla.Close()

	// Now check content of logFile
	// We wrote 5 entries,
	// 0 & 1 should be gone
	// 2 & 3 should be in the bak file
	// 4 should in in the log file
	if content, err := os.ReadFile(logFile); err != nil {
		t.Errorf("Failed to read log file %s: %v", logFile, err)
	} else {
		expectedContent := []byte("write 4\n")
		if !bytes.Equal(content, expectedContent) {
			t.Errorf(
				"unexpected log content: expected %s got %s",
				string(expectedContent),
				string(content),
			)
		}
	}

	logFileBak := logFile + ".bak"
	if content, err := os.ReadFile(logFileBak); err != nil {
		t.Errorf("Failed to read log bak file %s: %v", logFileBak, err)
	} else {
		expectedContent := []byte("write 2\nwrite 3\n")
		if !bytes.Equal(content, expectedContent) {
			t.Errorf(
				"unexpected log bak content: expected %s, got %s",
				string(expectedContent),
				string(content),
			)
		}
	}
}

func TestFailOpen(t *testing.T) {
	logDir := t.TempDir()
	logFile := logDir + "/DoesNotExist/test.log"
	_, err := NewRollingLogAppender(logFile, 2)
	if err == nil {
		t.Errorf("Expected error for non existent path: %s", logFile)
	}
}

func TestFailRotate(t *testing.T) {
	logDir := t.TempDir()
	logFile := logDir + "/test.log"
	rla, err := NewRollingLogAppender(logFile, 2)
	if err != nil {
		t.Fatalf("Failed to call to NewRollingLogAppender failed: %v", err)
	}

	// Trigger an error on rotate by having the file already closed
	rla.output.Close()

	if err := rla.rotate(); err == nil {
		t.Errorf("Expected rotate to fail on closed file")
	}

	// Re-open the file and then make rotate fail on the rename
	// by changing the fileName
	rla.openFile()
	rla.fileName += ".doesnotexist"
	if err := rla.rotate(); err == nil {
		t.Errorf("Expected rotate to fail on rename fail")
	}

	rla.Close()
}
