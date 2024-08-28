package main

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"eric-odp-factory/internal/config"
	"eric-odp-factory/internal/dirwatcher"
)

const (
	infoCfg  = "[ { \"severity\": \"info\", \"container\": \"eric-odp-factory\" } ]"
	debugCfg = "[ { \"severity\": \"debug\", \"container\": \"eric-odp-factory\" } ]"

	invalidJSON = "["
	emptyJSON   = "[ {} ]"
)

func TestLogging(t *testing.T) {
	logCtlDir := t.TempDir()
	logCtrlFile := logCtlDir + "/logcontrol.json"
	os.WriteFile(logCtrlFile, []byte(infoCfg), 0o644)

	dirwatcher.Start()

	appConfig = &config.Config{LogControlFile: logCtrlFile}
	initLogging()

	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Errorf("TestLogging: Handler is not a JSONHandler: %v", handler)
	}

	if programLevel.Level() != slog.LevelInfo {
		t.Errorf("TestLogging: unexpected Level after init, expected %v, got %v", slog.LevelInfo, programLevel)
	}

	os.WriteFile(logCtrlFile, []byte(debugCfg), 0o644)
	debugSet := false
	// Note the logctl only updates once every 2 seconds
	for i := 0; i < 5 && !debugSet; i++ {
		time.Sleep(time.Second)
		debugSet = programLevel.Level() == slog.LevelDebug
	}
	if !debugSet {
		t.Errorf("TestLogging: unexpected Level after updating cfg file, expected %v, got %v", slog.LevelDebug, programLevel)
	}

	dirwatcher.Stop()
}

func TestUnmarshalBad(t *testing.T) {
	logCtlDir := t.TempDir()
	logCtrlFileInvalid := logCtlDir + "/invalid.json"

	os.WriteFile(logCtrlFileInvalid, []byte(invalidJSON), 0o644)
	if lc, err := getLogControl(logCtrlFileInvalid); err == nil {
		t.Errorf("Expected error parsing invalid.json: lc=%v", lc)
	}

	logCtrlFileMissing := logCtlDir + "/missing.json"
	os.WriteFile(logCtrlFileMissing, []byte(emptyJSON), 0o644)
	if lc, err := getLogControl(logCtrlFileMissing); err == nil {
		t.Errorf("Expected error parsing missing.json: lc=%v", lc)
	}
}
