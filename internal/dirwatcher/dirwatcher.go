package dirwatcher

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

const (
	Create uint32 = 1 << iota
	Write         = 2
	Remove        = 3
	Rename        = 4
	Chmod         = 5
)

var (
	watcher  *fsnotify.Watcher
	handlers map[string]func(string, uint32)
)

func Start() error {
	slog.Info("dirwatcher.Start")

	handlers = make(map[string]func(string, uint32), 0)

	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Unable to create fsnotify watcher", "err", err)

		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	go func() {
		slog.Info("dirwatcher started")
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					slog.Warn("dirwatcher events channel closed", "err", err)

					return
				}

				eventDir := filepath.Dir(event.Name)
				handler, handlerExists := handlers[eventDir]
				if handlerExists {
					handler(event.Name, uint32(event.Op))
				} else {
					slog.Warn("No handler found", "event", event)
				}
			case werr, ok := <-watcher.Errors:
				if !ok {
					slog.Info("dirwatcher stopping")
					// Channel was closed (i.e. Watcher.Close() was called).
					return
				}

				slog.Info("Error in watch", "err", werr)
			}
		}
	}()

	return nil
}

func Stop() {
	slog.Info("dirwatcher.Stop")
	if watcher != nil {
		watcher.Close()
	}
}

func WatchDirectory(dir string, handler func(string, uint32)) error {
	err := watcher.Add(dir)
	if err == nil {
		handlers[dir] = handler

		return nil
	}

	return fmt.Errorf("failed to add watch for %s: %w", dir, err)
}
