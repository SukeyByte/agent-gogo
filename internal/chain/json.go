package chain

import "github.com/SukeyByte/agent-gogo/internal/textutil"

func sortedUnique(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}
