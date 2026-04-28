package chain

import "github.com/sukeke/agent-gogo/internal/textutil"

func decodeJSONObject(text string, target any) error {
	return textutil.DecodeJSONObject(text, target)
}

func sortedUnique(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}
