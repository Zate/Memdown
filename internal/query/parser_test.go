package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryParser(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantAST *QueryAST
		wantErr bool
	}{
		{
			name:  "simple type predicate",
			input: "type:fact",
			wantAST: &QueryAST{
				Type:  "predicate",
				Key:   "type",
				Value: "fact",
			},
		},
		{
			name:  "simple tag predicate",
			input: "tag:project:ctx",
			wantAST: &QueryAST{
				Type:  "predicate",
				Key:   "tag",
				Value: "project:ctx",
			},
		},
		{
			name:  "AND expression",
			input: "type:fact AND tag:project:ctx",
			wantAST: &QueryAST{
				Type: "and",
				Left: &QueryAST{
					Type: "predicate", Key: "type", Value: "fact",
				},
				Right: &QueryAST{
					Type: "predicate", Key: "tag", Value: "project:ctx",
				},
			},
		},
		{
			name:  "OR expression",
			input: "type:fact OR type:decision",
			wantAST: &QueryAST{
				Type: "or",
				Left: &QueryAST{
					Type: "predicate", Key: "type", Value: "fact",
				},
				Right: &QueryAST{
					Type: "predicate", Key: "type", Value: "decision",
				},
			},
		},
		{
			name:  "NOT expression",
			input: "NOT type:fact",
			wantAST: &QueryAST{
				Type: "not",
				Child: &QueryAST{
					Type: "predicate", Key: "type", Value: "fact",
				},
			},
		},
		{
			name:  "created time filter - relative",
			input: "created:>24h",
			wantAST: &QueryAST{
				Type:     "predicate",
				Key:      "created",
				Operator: ">",
				Value:    "24h",
			},
		},
		{
			name:  "tokens filter",
			input: "tokens:<1000",
			wantAST: &QueryAST{
				Type:     "predicate",
				Key:      "tokens",
				Operator: "<",
				Value:    "1000",
			},
		},
		{
			name:  "has predicate",
			input: "has:summary",
			wantAST: &QueryAST{
				Type:  "predicate",
				Key:   "has",
				Value: "summary",
			},
		},
		{
			name:    "empty query",
			input:   "",
			wantAST: nil,
		},
		{
			name:    "malformed - missing value",
			input:   "type:",
			wantErr: true,
		},
		{
			name:    "malformed - unclosed paren",
			input:   "(type:fact",
			wantErr: true,
		},
		{
			name:    "malformed - unknown key",
			input:   "unknown:value",
			wantErr: true,
		},
		{
			name:  "tag with tier namespace",
			input: "tag:tier:reference",
			wantAST: &QueryAST{
				Type:  "predicate",
				Key:   "tag",
				Value: "tier:reference",
			},
		},
		{
			name:  "complex query",
			input: "type:fact AND (tag:tier:reference OR tag:tier:working)",
		},
		{
			name:  "complex with NOT",
			input: "(type:fact OR type:decision) AND NOT tag:archived",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := Parse(tc.input)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tc.wantAST != nil {
				assert.Equal(t, tc.wantAST, ast)
			}
		})
	}
}
