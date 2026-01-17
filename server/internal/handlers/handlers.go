package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/zhaobenny/cctop/server/internal/auth"
	"github.com/zhaobenny/cctop/server/internal/database"
)

// Handler holds dependencies for HTTP handlers
type Handler struct {
	db         *database.DB
	sessionMgr *scs.SessionManager
	templates  *template.Template
}

// New creates a new Handler
func New(db *database.DB, sessionMgr *scs.SessionManager, templates *template.Template) *Handler {
	return &Handler{
		db:         db,
		sessionMgr: sessionMgr,
		templates:  templates,
	}
}

// Index handles the main page
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	userID := h.sessionMgr.GetString(r.Context(), "userID")

	if userID == "" {
		// Not logged in - show auth page
		h.templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
			"Content": "auth",
		})
		return
	}

	// Logged in - show dashboard
	user, err := h.db.GetUserByID(userID)
	if err != nil || user == nil {
		h.sessionMgr.Destroy(r.Context())
		h.templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
			"Content": "auth",
		})
		return
	}

	// Default view is monthly
	view := "monthly"
	usage, _ := h.db.GetUsageByMonth(userID)
	total, _ := h.db.GetTotalUsage(userID, 0)

	// Build server URL from request
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	serverURL := scheme + "://" + r.Host

	// Calculate billing period
	periodStart, periodEnd := database.GetBillingPeriod(user.BillingDay)

	h.templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"Content":     "dashboard",
		"User":        user,
		"Usage":       usage,
		"Total":       total,
		"ServerURL":   serverURL,
		"HasData":     len(usage) > 0,
		"View":        view,
		"BillingDay":  user.BillingDay,
		"PeriodStart": periodStart,
		"PeriodEnd":   periodEnd,
	})
}

// PartialAuth returns the auth form fragment
func (h *Handler) PartialAuth(w http.ResponseWriter, r *http.Request) {
	h.templates.ExecuteTemplate(w, "auth.html", nil)
}

// Login handles user login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "Invalid form data")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.renderError(w, "Username and password are required")
		return
	}

	user, err := h.db.GetUserByUsername(username)
	if err != nil {
		h.renderError(w, "An error occurred")
		return
	}

	if user == nil || !auth.CheckPassword(password, user.PasswordHash) {
		h.renderError(w, "Invalid username or password")
		return
	}

	// Create session
	h.sessionMgr.Put(r.Context(), "userID", user.ID)

	// Return dashboard fragment
	h.renderDashboard(w, user)
}

// Register handles user registration
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderError(w, "Invalid form data")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.renderError(w, "Username and password are required")
		return
	}

	if len(username) < 3 {
		h.renderError(w, "Username must be at least 3 characters")
		return
	}

	if len(password) < 8 {
		h.renderError(w, "Password must be at least 8 characters")
		return
	}

	// Check if username exists
	existing, _ := h.db.GetUserByUsername(username)
	if existing != nil {
		h.renderError(w, "Username already taken")
		return
	}

	// Create user
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		h.renderError(w, "An error occurred")
		return
	}

	userID, err := auth.GenerateID()
	if err != nil {
		h.renderError(w, "An error occurred")
		return
	}

	apiKey, err := auth.GenerateAPIKey()
	if err != nil {
		h.renderError(w, "An error occurred")
		return
	}

	user := &database.User{
		ID:           userID,
		Username:     username,
		PasswordHash: passwordHash,
		APIKey:       apiKey,
		CreatedAt:    time.Now(),
	}

	if err := h.db.CreateUser(user); err != nil {
		h.renderError(w, "Failed to create account")
		return
	}

	// Create session
	h.sessionMgr.Put(r.Context(), "userID", user.ID)

	// Return dashboard fragment
	h.renderDashboard(w, user)
}

// Logout handles user logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessionMgr.Destroy(r.Context())
	// Redirect to refresh the full page (header needs to hide username/logout)
	w.Header().Set("HX-Redirect", "/")
}

// PartialDashboard returns the dashboard fragment
func (h *Handler) PartialDashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		h.templates.ExecuteTemplate(w, "auth.html", nil)
		return
	}
	h.renderDashboard(w, user)
}

// PartialUsageTable returns the usage table fragment
func (h *Handler) PartialUsageTable(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	view := r.URL.Query().Get("view")
	if view == "" {
		view = "monthly" // default
	}

	var usage []database.AggregatedUsage
	var total *database.AggregatedUsage

	switch view {
	case "monthly":
		usage, _ = h.db.GetUsageByMonth(user.ID)
		total, _ = h.db.GetTotalUsage(user.ID, 0)
	case "billing":
		usage, _ = h.db.GetUsageByBillingCycle(user.ID, user.BillingDay)
		total, _ = h.db.GetTotalUsage(user.ID, 0)
	default: // daily
		usage, _ = h.db.GetUsageByDay(user.ID, 0)
		total, _ = h.db.GetTotalUsage(user.ID, 0)
	}

	periodStart, periodEnd := database.GetBillingPeriod(user.BillingDay)

	h.templates.ExecuteTemplate(w, "usage-table.html", map[string]interface{}{
		"Usage":       usage,
		"Total":       total,
		"View":        view,
		"BillingDay":  user.BillingDay,
		"PeriodStart": periodStart,
		"PeriodEnd":   periodEnd,
	})
}

// UpdateBillingDay handles billing day updates
func (h *Handler) UpdateBillingDay(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, "Invalid form data")
		return
	}

	billingDayStr := strings.TrimSpace(r.FormValue("billing_day"))

	var billingDay int
	if billingDayStr != "" {
		var err error
		billingDay, err = strconv.Atoi(billingDayStr)
		if err != nil {
			h.renderError(w, "Invalid billing day")
			return
		}
		// Clamp to valid range
		if billingDay > 31 {
			billingDay = 31
		} else if billingDay < 1 {
			billingDay = 1
		}
	}

	if err := h.db.UpdateUserBillingDay(user.ID, billingDay); err != nil {
		h.renderError(w, "Failed to update billing day")
		return
	}

	// Update user object and rebuild cycle summaries (cycle periods changed)
	user.BillingDay = billingDay
	h.db.RebuildCycleSummaries(user.ID, billingDay)

	// Return updated billing section
	h.templates.ExecuteTemplate(w, "billing-section.html", map[string]interface{}{
		"BillingDay": billingDay,
	})
}

// SyncRequest represents the incoming sync data
type SyncRequest struct {
	ClientID   string       `json:"client_id"`
	ClientName string       `json:"client_name"`
	Records    []SyncRecord `json:"records"`
}

// SyncRecord represents a single usage record in the sync request
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
}

// APISync handles the sync endpoint
func (h *Handler) APISync(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ClientID == "" {
		h.jsonError(w, "client_id is required", http.StatusBadRequest)
		return
	}

	if len(req.Records) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncResponse{
			Success:  true,
			Message:  "No records to sync",
			Inserted: 0,
		})
		return
	}

	// Get or create client
	clientName := req.ClientName
	if clientName == "" {
		clientName = req.ClientID
	}
	_, err := h.db.GetOrCreateClient(user.ID, req.ClientID, clientName)
	if err != nil {
		h.jsonError(w, "Failed to create client", http.StatusInternalServerError)
		return
	}

	// Convert to database records
	var records []database.UsageRecord
	for _, r := range req.Records {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}

		records = append(records, database.UsageRecord{
			UserID:              user.ID,
			ClientID:            req.ClientID,
			Timestamp:           ts,
			SessionID:           r.SessionID,
			ProjectPath:         r.ProjectPath,
			Model:               r.Model,
			InputTokens:         r.InputTokens,
			OutputTokens:        r.OutputTokens,
			CacheCreationTokens: r.CacheCreationTokens,
			CacheReadTokens:     r.CacheReadTokens,
		})
	}

	inserted, err := h.db.InsertUsageRecords(records)
	if err != nil {
		h.jsonError(w, "Failed to insert records", http.StatusInternalServerError)
		return
	}

	// Update summaries for affected periods only
	if inserted > 0 {
		h.db.UpdateSummaries(user.ID, user.BillingDay, records)
	}

	// Update last sync time
	h.db.UpdateClientLastSync(req.ClientID, time.Now())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SyncResponse{
		Success:  true,
		Message:  "Sync completed",
		Inserted: inserted,
	})
}

// SyncStatusResponse represents the sync status response
type SyncStatusResponse struct {
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
}

// APISyncStatus returns the sync status for a client
func (h *Handler) APISyncStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		h.jsonError(w, "client_id is required", http.StatusBadRequest)
		return
	}

	lastSync, err := h.db.GetClientSyncStatus(user.ID, clientID)
	if err != nil {
		h.jsonError(w, "Failed to get sync status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SyncStatusResponse{
		LastSyncAt: lastSync,
	})
}

func (h *Handler) renderDashboard(w http.ResponseWriter, user *database.User) {
	// Redirect to refresh the full page (header needs to update with username/logout)
	w.Header().Set("HX-Redirect", "/")
}

func (h *Handler) renderError(w http.ResponseWriter, message string) {
	h.templates.ExecuteTemplate(w, "error.html", map[string]interface{}{
		"Error": message,
	})
}

func (h *Handler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Health handles the health check endpoint
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	if err := h.db.Ping(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": "database unavailable"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
