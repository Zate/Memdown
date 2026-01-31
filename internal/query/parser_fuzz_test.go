package query

import (
	"testing"
)

func FuzzQueryParser(f *testing.F) {
	f.Add("type:fact")
	f.Add("tag:project:x AND tag:tier:reference")
	f.Add("(type:fact OR type:decision) AND NOT tag:archived")
	f.Add("")
	f.Add("type:fact AND type:fact AND type:fact")
	f.Add("((((type:fact))))")
	f.Add("created:>24h")
	f.Add("tokens:<1000")
	f.Add("has:summary")

	f.Fuzz(func(t *testing.T, input string) {
		// Should never panic, regardless of input
		_, _ = Parse(input)
	})
}
