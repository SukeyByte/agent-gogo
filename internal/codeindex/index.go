package codeindex

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type FileSummary struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Bytes    int64  `json:"bytes"`
	Lines    int    `json:"lines"`
}

type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Language string `json:"language"`
}

type Index struct {
	Root        string         `json:"root"`
	Files       []FileSummary  `json:"files"`
	Symbols     []Symbol       `json:"symbols"`
	LanguageMap map[string]int `json:"language_map"`
}

type Options struct {
	MaxFiles int
}

func Build(ctx context.Context, root string, options Options) (Index, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Index{}, err
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = 2000
	}
	index := Index{
		Root:        absRoot,
		Files:       []FileSummary{},
		Symbols:     []Symbol{},
		LanguageMap: map[string]int{},
	}
	err = filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(index.Files) >= options.MaxFiles || shouldSkipFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel := relPath(absRoot, path)
		language := languageForPath(path)
		lines, symbols := inspectFile(path, rel, language)
		index.Files = append(index.Files, FileSummary{
			Path:     rel,
			Language: language,
			Bytes:    info.Size(),
			Lines:    lines,
		})
		index.Symbols = append(index.Symbols, symbols...)
		index.LanguageMap[language]++
		return nil
	})
	if err != nil {
		return Index{}, err
	}
	sort.SliceStable(index.Files, func(i, j int) bool {
		return index.Files[i].Path < index.Files[j].Path
	})
	sort.SliceStable(index.Symbols, func(i, j int) bool {
		if index.Symbols[i].Path != index.Symbols[j].Path {
			return index.Symbols[i].Path < index.Symbols[j].Path
		}
		if index.Symbols[i].Line != index.Symbols[j].Line {
			return index.Symbols[i].Line < index.Symbols[j].Line
		}
		return index.Symbols[i].Name < index.Symbols[j].Name
	})
	return index, nil
}

func (i Index) SearchSymbols(query string, pathFilter string, limit int) []Symbol {
	query = strings.ToLower(strings.TrimSpace(query))
	pathFilter = filepath.ToSlash(strings.TrimSpace(pathFilter))
	result := make([]Symbol, 0)
	for _, symbol := range i.Symbols {
		if pathFilter != "" && !strings.Contains(symbol.Path, pathFilter) {
			continue
		}
		haystack := strings.ToLower(symbol.Name + " " + symbol.Kind + " " + symbol.Path)
		if query == "" || strings.Contains(haystack, query) {
			result = append(result, symbol)
		}
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func inspectFile(path string, rel string, language string) (int, []Symbol) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil
	}
	defer file.Close()
	var symbols []Symbol
	lineNo := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		lineNo++
		for _, symbol := range symbolsForLine(scanner.Text(), language) {
			symbol.Path = rel
			symbol.Line = lineNo
			symbol.Language = language
			symbols = append(symbols, symbol)
		}
	}
	return lineNo, symbols
}

var (
	goFuncPattern    = regexp.MustCompile(`^\s*func\s+(?:\([^)]+\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	goTypePattern    = regexp.MustCompile(`^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface|[A-Za-z_])`)
	jsFuncPattern    = regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	jsConstPattern   = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?\(?[^=]*=>`)
	jsClassPattern   = regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	cssRulePattern   = regexp.MustCompile(`^\s*([.#][A-Za-z0-9_-]+)\s*\{`)
	htmlIDPattern    = regexp.MustCompile(`\sid=["']([^"']+)["']`)
	htmlClassPattern = regexp.MustCompile(`\sclass=["']([^"']+)["']`)
)

func symbolsForLine(line string, language string) []Symbol {
	switch language {
	case "go":
		if match := goFuncPattern.FindStringSubmatch(line); len(match) == 2 {
			return []Symbol{{Name: match[1], Kind: "function"}}
		}
		if match := goTypePattern.FindStringSubmatch(line); len(match) >= 3 {
			return []Symbol{{Name: match[1], Kind: "type"}}
		}
	case "javascript", "typescript":
		if match := jsFuncPattern.FindStringSubmatch(line); len(match) == 2 {
			return []Symbol{{Name: match[1], Kind: "function"}}
		}
		if match := jsConstPattern.FindStringSubmatch(line); len(match) == 2 {
			return []Symbol{{Name: match[1], Kind: "function"}}
		}
		if match := jsClassPattern.FindStringSubmatch(line); len(match) == 2 {
			return []Symbol{{Name: match[1], Kind: "class"}}
		}
	case "css":
		if match := cssRulePattern.FindStringSubmatch(line); len(match) == 2 {
			return []Symbol{{Name: match[1], Kind: "selector"}}
		}
	case "html":
		var symbols []Symbol
		if match := htmlIDPattern.FindStringSubmatch(line); len(match) == 2 {
			symbols = append(symbols, Symbol{Name: "#" + match[1], Kind: "id"})
		}
		if match := htmlClassPattern.FindStringSubmatch(line); len(match) == 2 {
			for _, className := range strings.Fields(match[1]) {
				symbols = append(symbols, Symbol{Name: "." + className, Kind: "class"})
			}
		}
		return symbols
	}
	return nil
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".jsx":
		return "javascript"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	default:
		return "text"
	}
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", ".cache", "data", "logs", "dist":
		return true
	default:
		return false
	}
}

func shouldSkipFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1_000_000 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".sqlite", ".db", ".pdf", ".woff", ".ttf":
		return true
	default:
		return false
	}
}

func relPath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
