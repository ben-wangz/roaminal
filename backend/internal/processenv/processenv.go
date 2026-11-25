package processenv

import (
	"os"
	"strings"
)

// Filter removes backend-only Roaminal configuration from a child process.
// Callers explicitly add only the variables their child requires. In
// particular, this prevents database credentials, login passwords and push
// credentials from being inherited by terminal shells and helper programs.
func Filter(base []string) []string {
	result := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(key, "ROAMINAL_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func Environment() []string { return Filter(os.Environ()) }
