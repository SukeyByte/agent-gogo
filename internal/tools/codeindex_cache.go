package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SukeyByte/agent-gogo/internal/codeindex"
)

type codeIndexCache struct {
	mu      sync.Mutex
	entries map[string]codeindex.Index
	path    string
}

func newCodeIndexCache() *codeIndexCache {
	cache := &codeIndexCache{
		entries: map[string]codeindex.Index{},
		path:    codeIndexCachePath(),
	}
	cache.load()
	return cache
}

func (c *codeIndexCache) get(ctx context.Context, root string, maxFiles int) (codeindex.Index, bool, error) {
	if c == nil {
		index, err := codeindex.Build(ctx, root, codeindex.Options{MaxFiles: maxFiles})
		return index, false, err
	}
	key := codeIndexCacheKey(root, maxFiles)
	c.mu.Lock()
	if index, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return index, true, nil
	}
	c.mu.Unlock()
	index, err := codeindex.Build(ctx, root, codeindex.Options{MaxFiles: maxFiles})
	if err != nil {
		return codeindex.Index{}, false, err
	}
	c.mu.Lock()
	c.entries[key] = index
	_ = c.saveLocked()
	c.mu.Unlock()
	return index, false, nil
}

func (c *codeIndexCache) invalidate(root string) {
	if c == nil {
		return
	}
	rootKey, err := filepath.Abs(root)
	if err != nil {
		rootKey = root
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key == rootKey || len(key) > len(rootKey) && key[:len(rootKey)+1] == rootKey+":" {
			delete(c.entries, key)
		}
	}
	_ = c.saveLocked()
}

func (c *codeIndexCache) load() {
	if c == nil || strings.TrimSpace(c.path) == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var disk struct {
		Version int                        `json:"version"`
		Entries map[string]codeindex.Index `json:"entries"`
	}
	if err := json.Unmarshal(data, &disk); err != nil || disk.Version != 1 {
		return
	}
	if disk.Entries != nil {
		c.entries = disk.Entries
	}
}

func (c *codeIndexCache) saveLocked() error {
	if c == nil || strings.TrimSpace(c.path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Version int                        `json:"version"`
		Entries map[string]codeindex.Index `json:"entries"`
	}{Version: 1, Entries: c.entries}, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func codeIndexCachePath() string {
	path := strings.TrimSpace(os.Getenv("AGENT_GOGO_CODE_INDEX_CACHE"))
	if path == "off" || path == "false" {
		return ""
	}
	if path == "" {
		path = filepath.Join("data", "code_index.json")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func codeIndexCacheKey(root string, maxFiles int) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if maxFiles <= 0 {
		maxFiles = 2000
	}
	return fmt.Sprintf("%s:%d", absRoot, maxFiles)
}

func (r *Runtime) indexCode(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	maxFiles := intArg(args["max_files"], 2000)
	index, cacheHit, err := r.codeIndexCache.get(ctx, root, maxFiles)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"root":         index.Root,
		"file_count":   len(index.Files),
		"symbol_count": len(index.Symbols),
		"languages":    index.LanguageMap,
		"cache_hit":    cacheHit,
		"files":        limitFileSummaries(index.Files, intArg(args["limit"], 80)),
		"symbols":      limitSymbols(index.Symbols, intArg(args["symbol_limit"], 120)),
	}, nil
}

func (r *Runtime) codeSymbols(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	maxFiles := intArg(args["max_files"], 2000)
	index, cacheHit, err := r.codeIndexCache.get(ctx, root, maxFiles)
	if err != nil {
		return nil, err
	}
	query, _ := args["query"].(string)
	path, _ := args["path"].(string)
	limit := intArg(args["limit"], 80)
	symbols := index.SearchSymbols(query, path, limit)
	return map[string]any{
		"query":     query,
		"path":      path,
		"symbols":   symbols,
		"count":     len(symbols),
		"cache_hit": cacheHit,
	}, nil
}

func (r *Runtime) invalidateCodeIndex(root string) {
	r.codeIndexCache.invalidate(root)
}
