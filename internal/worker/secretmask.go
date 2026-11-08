package worker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// minMaskedSecret is the shortest secret value that is masked. A shorter one (a PIN, "true", "1234") would turn
// ordinary words and numbers in a summary into "***" and say little about the secret; a credential worth keeping out of
// kata is longer than this.
const minMaskedSecret = 8

const secretMask = "***"

// secretValues are the values of the secrets the run's steps may have printed: every secret_refs source, read again
// because the journal keeps no value (ADR-0009). A source that cannot be read now has nothing to mask, and a value
// under minMaskedSecret bytes is not masked. Longest first, so a value that contains another is masked whole.
func (w *Worker) secretValues(j *Journal) []string {
	var values []string
	for _, name := range j.SecretRefs {
		value, err := w.opt.Config.readSecret(name)
		if err == nil && len(value) >= minMaskedSecret && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	return values
}

// maskSecrets replaces every occurrence of each value with ***.
func maskSecrets(s string, values []string) string {
	for _, v := range values {
		s = strings.ReplaceAll(s, v, secretMask)
	}
	return s
}

// maskedCommandTail is the end of a command step's output for its summary, with the run's secrets masked in the whole log
// before the tail is cut, so a value the cut would split is still masked.
func maskedCommandTail(dir string, values []string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "output.log"))
	if err != nil {
		return ""
	}
	return tailOf(maskSecrets(string(raw), values), 1024)
}
