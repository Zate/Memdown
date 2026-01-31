package token

// Estimate returns a rough token estimate for the given content.
// Simple formula: len(content) / 4. Intentionally naive.
func Estimate(content string) int {
	n := len(content) / 4
	if n == 0 && len(content) > 0 {
		return 1
	}
	return n
}
