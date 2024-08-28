package main

import (
	"fmt"
	"os"
	"sync"
)

type RollingLogAppender struct {
	fileName string

	maxWrites int

	writes int
	output *os.File
	lock   sync.RWMutex
}

func NewRollingLogAppender(fileName string, maxWrites int) (*RollingLogAppender, error) {
	rla := &RollingLogAppender{
		fileName:  fileName,
		maxWrites: maxWrites,
		writes:    0,
		lock:      sync.RWMutex{},
	}

	if err := rla.openFile(); err != nil {
		return nil, err
	}

	return rla, nil
}

func (rla *RollingLogAppender) Close() {
	rla.output.Close()
}

func (rla *RollingLogAppender) Write(p []byte) (n int, err error) {
	rla.lock.Lock()
	defer rla.lock.Unlock()

	if rla.writes >= rla.maxWrites {
		if err := rla.rotate(); err != nil {
			return 0, err
		}
	}

	rla.writes++

	return rla.output.Write(p) //nolint:wrapcheck // We're just wrapping the Write
}

func (rla *RollingLogAppender) rotate() error {
	if err := rla.output.Close(); err != nil {
		return fmt.Errorf("failed to close current log: %w", err)
	}

	if err := os.Rename(rla.fileName, rla.fileName+".bak"); err != nil {
		return fmt.Errorf("failed to rotate log: %w", err)
	}

	return rla.openFile()
}

func (rla *RollingLogAppender) openFile() error {
	var err error
	if rla.output, err = os.Create(rla.fileName); err != nil {
		return fmt.Errorf("failed to open log %s: %w", rla.fileName, err)
	}

	rla.writes = 0

	return nil
}
