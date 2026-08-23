package textutil

import (
	"encoding/json"
	"sort"
	"strings"
)

func DecodeJSONObject(text string, target any) error {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end >= start {
		text = text[start : end+1]
	}
	if err := json.Unmarshal([]byte(text), target); err == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(repairSpacedEscapes(text)), target); err == nil {
		return nil
	}
	return json.Unmarshal([]byte(completeTruncatedJSON(text)), target)
}

// completeTruncatedJSON closes dangling strings, arrays, and objects left open
// by a truncated model response (e.g. output cut right before the final '}'),
// so a nearly-complete object can still be decoded deterministically instead
// of burning another LLM repair round-trip that often truncates the same way.
func completeTruncatedJSON(text string) string {
	var closers []byte
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			closers = append(closers, '}')
		case '[':
			closers = append(closers, ']')
		case '}', ']':
			if len(closers) > 0 {
				closers = closers[:len(closers)-1]
			}
		}
	}
	if len(closers) == 0 && !inString {
		return text
	}
	if inString {
		if escaped {
			// The string ended on a dangling escape; dropping it lets the
			// closing quote terminate the string instead of escaping it.
			text = text[:len(text)-1]
		}
		text += `"`
	}
	completed := strings.TrimRight(text, " \t\r\n")
	completed = strings.TrimSuffix(completed, ",")
	var closing strings.Builder
	for i := len(closers) - 1; i >= 0; i-- {
		closing.WriteByte(closers[i])
	}
	return completed + closing.String()
}

func SortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	var previous string
	for i, value := range result {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if i > 0 && value == previous {
			continue
		}
		out = append(out, value)
		previous = value
	}
	if out == nil {
		return []string{}
	}
	return out
}

func repairSpacedEscapes(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	inString := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '"' && !isEscaped(text, i) {
			inString = !inString
			builder.WriteByte(ch)
			continue
		}
		if inString && ch == '\\' {
			builder.WriteByte(ch)
			j := i + 1
			for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\r' || text[j] == '\n') {
				j++
			}
			if j < len(text) && isJSONEscape(text[j]) {
				builder.WriteByte(text[j])
				i = j
				continue
			}
			continue
		}
		builder.WriteByte(ch)
	}
	return builder.String()
}

func isEscaped(text string, index int) bool {
	count := 0
	for i := index - 1; i >= 0 && text[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func isJSONEscape(ch byte) bool {
	switch ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}
