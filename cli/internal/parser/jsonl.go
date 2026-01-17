package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/zhaobenny/cctop/internal/model"
)

// rawMessage represents the raw JSON structure from Claude Code JSONL files
type rawMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Message   struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// FindUsageFiles finds all JSONL files in the Claude projects directory
func FindUsageFiles() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	var files []string

	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// ParseFile parses a single JSONL file and returns usage records
func ParseFile(path string) ([]model.UsageRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []model.UsageRecord
	scanner := bufio.NewScanner(file)

	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw rawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			// Skip malformed lines
			continue
		}

		// Only process assistant messages with usage data
		if raw.Type != "assistant" || raw.Message.Model == "" {
			continue
		}

		// Skip if no actual usage
		usage := raw.Message.Usage
		if usage.InputTokens == 0 && usage.OutputTokens == 0 {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339, raw.Timestamp)
		if err != nil {
			continue
		}

		records = append(records, model.UsageRecord{
			Timestamp:   timestamp,
			SessionID:   raw.SessionID,
			ProjectPath: raw.CWD,
			Model:       raw.Message.Model,
			Usage: model.TokenUsage{
				InputTokens:              usage.InputTokens,
				OutputTokens:             usage.OutputTokens,
				CacheCreationInputTokens: usage.CacheCreationInputTokens,
				CacheReadInputTokens:     usage.CacheReadInputTokens,
			},
		})
	}

	return records, scanner.Err()
}

// ParseAllFiles parses all Claude Code JSONL files and returns all records
func ParseAllFiles() ([]model.UsageRecord, error) {
	files, err := FindUsageFiles()
	if err != nil {
		return nil, err
	}

	var allRecords []model.UsageRecord
	for _, file := range files {
		records, err := ParseFile(file)
		if err != nil {
			// Log error but continue with other files
			continue
		}
		allRecords = append(allRecords, records...)
	}

	return allRecords, nil
}
