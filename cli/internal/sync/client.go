package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/zhaobenny/cctop/cli/internal/config"
	"github.com/zhaobenny/cctop/internal/model"
)

// Client handles syncing to the server
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

// SyncRequest represents the sync API request body
type SyncRequest struct {
	ClientID   string       `json:"client_id"`
	ClientName string       `json:"client_name"`
	Records    []SyncRecord `json:"records"`
}

// SyncRecord represents a single usage record
type SyncRecord struct {
	Timestamp           string `json:"timestamp"`
	SessionID           string `json:"session_id"`
	ProjectPath         string `json:"project_path"`
	Model               string `json:"model"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
}

// SyncResponse represents the sync API response
type SyncResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message,omitempty"`
	Inserted int64  `json:"inserted,omitempty"`
	Error    string `json:"error,omitempty"`
}

// SyncStatusResponse represents the sync status response
type SyncStatusResponse struct {
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// NewClient creates a new sync client
func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetSyncStatus gets the last sync time from the server
func (c *Client) GetSyncStatus() (*time.Time, error) {
	url := fmt.Sprintf("%s/api/sync/status?client_id=%s", c.cfg.Server, c.cfg.ClientID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var status SyncStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	if status.Error != "" {
		return nil, fmt.Errorf("%s", status.Error)
	}

	return status.LastSyncAt, nil
}

// Sync sends usage records to the server
func (c *Client) Sync(records []model.UsageRecord) (int64, error) {
	// Get hostname for client name
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Convert to sync records
	syncRecords := make([]SyncRecord, len(records))
	for i, r := range records {
		syncRecords[i] = SyncRecord{
			Timestamp:           r.Timestamp.Format(time.RFC3339),
			SessionID:           r.SessionID,
			ProjectPath:         r.ProjectPath,
			Model:               r.Model,
			InputTokens:         r.Usage.InputTokens,
			OutputTokens:        r.Usage.OutputTokens,
			CacheCreationTokens: r.Usage.CacheCreationInputTokens,
			CacheReadTokens:     r.Usage.CacheReadInputTokens,
		}
	}

	reqBody := SyncRequest{
		ClientID:   c.cfg.ClientID,
		ClientName: hostname,
		Records:    syncRecords,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/api/sync", c.cfg.Server)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return 0, err
	}

	if !syncResp.Success {
		errMsg := syncResp.Error
		if errMsg == "" {
			errMsg = syncResp.Message
		}
		return 0, fmt.Errorf("%s", errMsg)
	}

	return syncResp.Inserted, nil
}
