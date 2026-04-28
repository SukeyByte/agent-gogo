package observability

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileLogger struct {
	path string
	mu   sync.Mutex
}

func NewFileLogger(dir string, prefix string) (*FileLogger, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "logs"
	}
	if strings.TrimSpace(prefix) == "" {
		prefix = "run"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := prefix + "-" + time.Now().UTC().Format("20060102T150405.000000Z") + ".jsonl"
	return &FileLogger{path: filepath.Join(dir, name)}, nil
}

func (l *FileLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *FileLogger) Log(ctx context.Context, stage string, payload any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil {
		return nil
	}
	record := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
		"stage":   stage,
		"payload": payload,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}
