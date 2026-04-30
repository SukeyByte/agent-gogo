package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/sukeke/agent-gogo/internal/codeindex"
)

type codeIndexCache struct {
	mu      sync.Mutex
	entries map[string]codeindex.Index
}

func newCodeIndexCache() *codeIndexCache {
	return &codeIndexCache{entries: map[string]codeindex.Index{}}
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
