package hook

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// readAssistantResponsesFromOffset reads a JSONL transcript file starting at
// the given byte offset and returns the concatenated text content of all
// assistant messages found after that offset, along with the new file offset.
func readAssistantResponsesFromOffset(path string, offset int64) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", offset, err
		}
	}

	var responses []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry["type"] == "assistant" {
			if msg, ok := entry["message"].(map[string]any); ok {
				switch content := msg["content"].(type) {
				case string:
					if content != "" {
						responses = append(responses, content)
					}
				case []any:
					for _, block := range content {
						if b, ok := block.(map[string]any); ok {
							if b["type"] == "text" {
								if text, ok := b["text"].(string); ok && text != "" {
									responses = append(responses, text)
								}
							}
						}
					}
				}
			}
		}
	}

	// Get current file position for the new cursor
	newOffset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return strings.Join(responses, "\n"), offset, err
	}

	return strings.Join(responses, "\n"), newOffset, nil
}

// readTranscriptPathFromStdin reads the hook input JSON from stdin and returns
// the transcript_path value.
func readTranscriptPathFromStdin() (string, error) {
	var input map[string]any
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		return "", err
	}
	if tp, ok := input["transcript_path"].(string); ok {
		return tp, nil
	}
	return "", nil
}
