package hook

import (
	"regexp"
	"strings"
)

// CtxCommand represents a parsed <ctx:*> command from Claude's response.
type CtxCommand struct {
	Type    string            `json:"type"`
	Attrs   map[string]string `json:"attrs,omitempty"`
	Content string            `json:"content,omitempty"`
}

var (
	// Match opening tags: <ctx:command attr="value" ...> or self-closing <ctx:command attr="value" .../>
	openTagRe  = regexp.MustCompile(`<ctx:(\w+)((?:\s+\w+="[^"]*")*)\s*/?>`)
	closeTagRe = regexp.MustCompile(`</ctx:(\w+)>`)
	attrRe     = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// ParseCtxCommands parses <ctx:*> commands from Claude's response.
// Commands inside code blocks (fenced or inline) are ignored.
func ParseCtxCommands(response string) []CtxCommand {
	// Find code block regions to exclude
	codeRegions := findCodeRegions(response)

	var commands []CtxCommand

	// Find all opening tags
	matches := openTagRe.FindAllStringIndex(response, -1)
	for _, match := range matches {
		start := match[0]

		// Skip if inside code block
		if isInCodeRegion(start, codeRegions) {
			continue
		}

		fullMatch := response[match[0]:match[1]]
		tagMatch := openTagRe.FindStringSubmatch(fullMatch)
		if tagMatch == nil {
			continue
		}

		cmdType := tagMatch[1]
		attrStr := tagMatch[2]

		attrs := parseAttrs(attrStr)

		// Check if self-closing
		if strings.HasSuffix(fullMatch, "/>") {
			cmd := CtxCommand{
				Type:  cmdType,
				Attrs: attrs,
			}
			if len(attrs) == 0 {
				cmd.Attrs = nil
			}
			commands = append(commands, cmd)
			continue
		}

		// Find closing tag
		closePattern := "</ctx:" + cmdType + ">"
		closeIdx := strings.Index(response[match[1]:], closePattern)
		if closeIdx == -1 {
			// Unclosed tag, skip
			continue
		}

		content := strings.TrimSpace(response[match[1] : match[1]+closeIdx])

		cmd := CtxCommand{
			Type:    cmdType,
			Attrs:   attrs,
			Content: content,
		}
		if len(attrs) == 0 {
			cmd.Attrs = nil
		}
		commands = append(commands, cmd)
	}

	if commands == nil {
		return []CtxCommand{}
	}
	return commands
}

func parseAttrs(s string) map[string]string {
	attrs := map[string]string{}
	matches := attrRe.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		attrs[m[1]] = m[2]
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

type codeRegion struct {
	start, end int
}

func findCodeRegions(text string) []codeRegion {
	var regions []codeRegion

	// Find fenced code blocks (```)
	i := 0
	for i < len(text) {
		idx := strings.Index(text[i:], "```")
		if idx == -1 {
			break
		}
		blockStart := i + idx
		// Find closing ```
		endIdx := strings.Index(text[blockStart+3:], "```")
		if endIdx == -1 {
			// Unclosed code block, treat rest as code
			regions = append(regions, codeRegion{blockStart, len(text)})
			break
		}
		blockEnd := blockStart + 3 + endIdx + 3
		regions = append(regions, codeRegion{blockStart, blockEnd})
		i = blockEnd
	}

	// Find inline code (`...`)
	i = 0
	for i < len(text) {
		// Skip already-found fenced blocks
		if isInCodeRegion(i, regions) {
			i++
			continue
		}

		if text[i] == '`' {
			// Find closing backtick (but not triple backtick)
			if i+1 < len(text) && text[i+1] == '`' {
				i++
				continue
			}
			end := strings.Index(text[i+1:], "`")
			if end == -1 {
				break
			}
			regions = append(regions, codeRegion{i, i + 1 + end + 1})
			i = i + 1 + end + 1
		} else {
			i++
		}
	}

	return regions
}

func isInCodeRegion(pos int, regions []codeRegion) bool {
	for _, r := range regions {
		if pos >= r.start && pos < r.end {
			return true
		}
	}
	return false
}
