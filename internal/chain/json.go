package chain

import "github.com/SukeyByte/agent-gogo/internal/textutil"

func decodeJSONObject(text string, target any) error {
	return textutil.DecodeJSONObject(text, target)
}

func sortedUnique(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}
