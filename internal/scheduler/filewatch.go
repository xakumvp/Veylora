package scheduler

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"veylora/internal/nodes"
)

const (
	fileDebounce   = 500 * time.Millisecond
	maxWatchedDirs = 500
)

type fileConfig struct {
	dir                          string
	pattern                      string
	onCreate, onModify, onDelete bool
	recursive                    bool
}

type fileWatcher struct {
	cfg fileConfig
	w   *fsnotify.Watcher
}

func newFileWatcher(cfg fileConfig) (*fileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("не удалось создать наблюдатель: %w", err)
	}
	fw := &fileWatcher{cfg: cfg, w: w}
	if err := fw.add(cfg.dir); err != nil {
		w.Close()
		return nil, err
	}
	return fw, nil
}

func (fw *fileWatcher) add(dir string) error {
	if !fw.cfg.recursive {
		return fw.w.Add(dir)
	}
	count := 0
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subdirectory: skip
		}
		if !d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return filepath.SkipDir // never follow links out of the allowed tree
		}
		if count >= maxWatchedDirs {
			return fmt.Errorf("слишком много подкаталогов (максимум %d)", maxWatchedDirs)
		}
		count++
		return fw.w.Add(path)
	})
}

// run delivers debounced, filtered events until ctx is cancelled.
func (fw *fileWatcher) run(ctx context.Context, logger *slog.Logger, emit func(map[string]any)) {
	defer fw.w.Close()
	var mu sync.Mutex
	pending := map[string]*time.Timer{}
	defer func() {
		mu.Lock()
		for _, t := range pending {
			t.Stop()
		}
		mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fw.w.Errors:
			if !ok {
				return
			}
			logger.Warn("file watcher error", "dir", fw.cfg.dir, "error", err)
		case ev, ok := <-fw.w.Events:
			if !ok {
				return
			}
			kind := ""
			switch {
			case ev.Has(fsnotify.Create):
				kind = nodes.FileCreated
				if fw.cfg.recursive {
					if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
						_ = fw.add(ev.Name)
						continue
					}
				}
			case ev.Has(fsnotify.Write):
				kind = nodes.FileModified
			case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
				kind = nodes.FileDeleted
			default:
				continue
			}
			if (kind == nodes.FileCreated && !fw.cfg.onCreate) || (kind == nodes.FileModified && !fw.cfg.onModify) || (kind == nodes.FileDeleted && !fw.cfg.onDelete) {
				continue
			}
			name := filepath.Base(ev.Name)
			if ok, _ := filepath.Match(fw.cfg.pattern, name); !ok {
				continue
			}
			if kind != nodes.FileDeleted {
				if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
					continue
				}
			}
			// Editors emit bursts of events per save: keep only the last one per path.
			key := ev.Name
			data := map[string]any{"event": kind, "path": ev.Name, "name": name, "directory": filepath.Dir(ev.Name)}
			mu.Lock()
			if t, ok := pending[key]; ok {
				t.Stop()
			}
			pending[key] = time.AfterFunc(fileDebounce, func() {
				mu.Lock()
				delete(pending, key)
				mu.Unlock()
				if ctx.Err() == nil {
					emit(data)
				}
			})
			mu.Unlock()
		}
	}
}
