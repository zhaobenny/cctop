package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zhaobenny/cctop/internal/model"
	"github.com/zhaobenny/cctop/internal/pricing"
)

// DB wraps the SQL database connection
type DB struct {
	*sql.DB
}

// User represents a user account
type User struct {
	ID           string
	Username     string
	PasswordHash string
	APIKey       string
	BillingDay   int // Day of month (1-31), 0 = disabled
	CreatedAt    time.Time
}

// Client represents a sync client
type Client struct {
	ID         string
	UserID     string
	Name       string
	LastSyncAt *time.Time
	CreatedAt  time.Time
}

// UsageRecord represents a usage record from Claude Code
type UsageRecord struct {
	ID                  int64
	UserID              string
	ClientID            string
	Timestamp           time.Time
	SessionID           string
	ProjectPath         string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// Open opens a SQLite database connection
func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to avoid "database is locked" errors under concurrent load
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	return &DB{db}, nil
}

// Migrate creates the database schema
func (db *DB) Migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		api_key TEXT UNIQUE NOT NULL,
		billing_day INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		last_sync_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT NOT NULL,
		client_id TEXT NOT NULL,
		timestamp TIMESTAMP NOT NULL,
		session_id TEXT NOT NULL,
		project_path TEXT,
		model TEXT NOT NULL,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cache_creation_tokens INTEGER DEFAULT 0,
		cache_read_tokens INTEGER DEFAULT 0,
		cost REAL DEFAULT 0,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(user_id, client_id, timestamp, session_id, model)
	);

	CREATE INDEX IF NOT EXISTS idx_usage_user_timestamp ON usage_records(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_clients_user ON clients(user_id);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		data BLOB NOT NULL,
		expiry REAL NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expiry);

	CREATE TABLE IF NOT EXISTS usage_summary (
		user_id TEXT NOT NULL,
		period_type TEXT NOT NULL,
		period_key TEXT NOT NULL,
		period_start TIMESTAMP NOT NULL,
		period_end TIMESTAMP NOT NULL,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cache_creation_tokens INTEGER NOT NULL,
		cache_read_tokens INTEGER NOT NULL,
		cost REAL DEFAULT 0,
		PRIMARY KEY (user_id, period_type, period_key),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_summary_user_type ON usage_summary(user_id, period_type);
	`

	_, err := db.Exec(schema)
	return err
}

// CreateUser creates a new user
func (db *DB) CreateUser(user *User) error {
	_, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, api_key, billing_day, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.APIKey, user.BillingDay, user.CreatedAt,
	)
	return err
}

// GetUserByUsername retrieves a user by username
func (db *DB) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT id, username, password_hash, api_key, billing_day, created_at
		 FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIKey, &user.BillingDay, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID retrieves a user by ID
func (db *DB) GetUserByID(id string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT id, username, password_hash, api_key, billing_day, created_at
		 FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIKey, &user.BillingDay, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByAPIKey retrieves a user by API key
func (db *DB) GetUserByAPIKey(apiKey string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		`SELECT id, username, password_hash, api_key, billing_day, created_at
		 FROM users WHERE api_key = ?`,
		apiKey,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIKey, &user.BillingDay, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateUserBillingDay updates a user's billing day
func (db *DB) UpdateUserBillingDay(userID string, billingDay int) error {
	_, err := db.Exec(`UPDATE users SET billing_day = ? WHERE id = ?`, billingDay, userID)
	return err
}

// GetOrCreateClient gets an existing client or creates a new one
func (db *DB) GetOrCreateClient(userID, clientID, clientName string) (*Client, error) {
	// Try to get existing client
	client := &Client{}
	var lastSyncAt sql.NullTime
	err := db.QueryRow(
		`SELECT id, user_id, name, last_sync_at, created_at FROM clients WHERE id = ? AND user_id = ?`,
		clientID, userID,
	).Scan(&client.ID, &client.UserID, &client.Name, &lastSyncAt, &client.CreatedAt)

	if err == nil {
		if lastSyncAt.Valid {
			client.LastSyncAt = &lastSyncAt.Time
		}
		return client, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	// Create new client
	now := time.Now()
	_, err = db.Exec(
		`INSERT INTO clients (id, user_id, name, created_at) VALUES (?, ?, ?, ?)`,
		clientID, userID, clientName, now,
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		CreatedAt: now,
	}, nil
}

// UpdateClientLastSync updates the last sync time for a client
func (db *DB) UpdateClientLastSync(clientID string, lastSyncAt time.Time) error {
	_, err := db.Exec(`UPDATE clients SET last_sync_at = ? WHERE id = ?`, lastSyncAt, clientID)
	return err
}

// InsertUsageRecords inserts multiple usage records, ignoring duplicates
func (db *DB) InsertUsageRecords(records []UsageRecord) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO usage_records
		(user_id, client_id, timestamp, session_id, project_path, model,
		 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var inserted int64
	for _, r := range records {
		// Calculate cost using shared pricing module
		modelPricing := pricing.GetPricing(r.Model, true) // offline mode for server
		cost := pricing.CalculateCost(model.TokenUsage{
			InputTokens:              r.InputTokens,
			OutputTokens:             r.OutputTokens,
			CacheCreationInputTokens: r.CacheCreationTokens,
			CacheReadInputTokens:     r.CacheReadTokens,
		}, modelPricing)
		result, err := stmt.Exec(
			r.UserID, r.ClientID, r.Timestamp, r.SessionID, r.ProjectPath, r.Model,
			r.InputTokens, r.OutputTokens, r.CacheCreationTokens, r.CacheReadTokens, cost,
		)
		if err != nil {
			return 0, err
		}
		n, _ := result.RowsAffected()
		inserted += n
	}

	return inserted, tx.Commit()
}

// AggregatedUsage represents aggregated usage data
type AggregatedUsage struct {
	Period              string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	Cost                float64
}

// clampDay returns the billing day clamped to the last day of the given month
func clampDay(year int, month time.Month, day int) int {
	// Get last day of month by going to next month day 0
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > lastDay {
		return lastDay
	}
	return day
}

// GetBillingPeriod calculates the current billing period based on billing day
// Returns (periodStart, periodEnd) dates. If billingDay is 0, returns zero times.
// Handles months with fewer days by clamping (e.g., day 31 in Feb becomes Feb 28/29)
func GetBillingPeriod(billingDay int) (time.Time, time.Time) {
	if billingDay <= 0 || billingDay > 31 {
		return time.Time{}, time.Time{}
	}

	now := time.Now()
	year, month, day := now.Date()

	// Calculate period start - clamp to valid day for the month
	var periodStart time.Time
	if day >= clampDay(year, month, billingDay) {
		// Current period started this month
		clampedDay := clampDay(year, month, billingDay)
		periodStart = time.Date(year, month, clampedDay, 0, 0, 0, 0, now.Location())
	} else {
		// Current period started last month
		prevMonth := month - 1
		prevYear := year
		if prevMonth < 1 {
			prevMonth = 12
			prevYear--
		}
		clampedDay := clampDay(prevYear, prevMonth, billingDay)
		periodStart = time.Date(prevYear, prevMonth, clampedDay, 0, 0, 0, 0, now.Location())
	}

	// Period end is one month after start, also clamped
	endYear, endMonth := year, month+1
	if day < clampDay(year, month, billingDay) {
		endMonth = month
	}
	if endMonth > 12 {
		endMonth = 1
		endYear++
	}
	clampedEndDay := clampDay(endYear, endMonth, billingDay)
	periodEnd := time.Date(endYear, endMonth, clampedEndDay, 0, 0, 0, 0, now.Location()).Add(-time.Second)

	return periodStart, periodEnd
}

// GetUsageByDay returns daily usage for a user, optionally filtered by billing period
func (db *DB) GetUsageByDay(userID string, billingDay int) ([]AggregatedUsage, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	periodStart, _ := GetBillingPeriod(billingDay)

	var results []AggregatedUsage

	// Get completed days from summary table
	summaryQuery := `
		SELECT period_key, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost
		FROM usage_summary
		WHERE user_id = ? AND period_type = 'day' AND period_key != ?
	`
	args := []interface{}{userID, today}
	if !periodStart.IsZero() {
		summaryQuery += ` AND period_start >= ?`
		args = append(args, periodStart)
	}
	summaryQuery += ` ORDER BY period_key DESC LIMIT 30`

	rows, err := db.Query(summaryQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var u AggregatedUsage
		if err := rows.Scan(&u.Period, &u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens, &u.Cost); err != nil {
			return nil, err
		}
		results = append(results, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get today's data from raw records
	var todayUsage AggregatedUsage
	todayUsage.Period = today
	err = db.QueryRow(`
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cost), 0)
		FROM usage_records
		WHERE user_id = ? AND DATE(timestamp) = ?
	`, userID, today).Scan(&todayUsage.InputTokens, &todayUsage.OutputTokens, &todayUsage.CacheCreationTokens, &todayUsage.CacheReadTokens, &todayUsage.Cost)
	if err != nil {
		return nil, err
	}

	// Only include today if there's data
	if todayUsage.InputTokens > 0 || todayUsage.OutputTokens > 0 {
		results = append([]AggregatedUsage{todayUsage}, results...)
	}

	return results, nil
}

// GetUsageByBillingCycle returns usage grouped by billing cycles
func (db *DB) GetUsageByBillingCycle(userID string, billingDay int) ([]AggregatedUsage, error) {
	if billingDay <= 0 || billingDay > 31 {
		return nil, nil
	}

	// Get current cycle info
	cycleStart, cycleEnd := GetBillingPeriod(billingDay)
	currentCycleKey := cycleStart.Format("Jan 2") + " – " + cycleEnd.Format("Jan 2")

	var results []AggregatedUsage

	// Get completed cycles from summary table (where period_end < now)
	rows, err := db.Query(`
		SELECT period_key, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost
		FROM usage_summary
		WHERE user_id = ? AND period_type = 'cycle' AND period_key != ?
		ORDER BY period_start DESC
	`, userID, currentCycleKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var u AggregatedUsage
		if err := rows.Scan(&u.Period, &u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens, &u.Cost); err != nil {
			return nil, err
		}
		results = append(results, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get current cycle's data from raw records
	var currentUsage AggregatedUsage
	currentUsage.Period = currentCycleKey
	err = db.QueryRow(`
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cost), 0)
		FROM usage_records
		WHERE user_id = ? AND timestamp >= ? AND timestamp <= ?
	`, userID, cycleStart, cycleEnd).Scan(&currentUsage.InputTokens, &currentUsage.OutputTokens, &currentUsage.CacheCreationTokens, &currentUsage.CacheReadTokens, &currentUsage.Cost)
	if err != nil {
		return nil, err
	}

	// Only include current cycle if there's data
	if currentUsage.InputTokens > 0 || currentUsage.OutputTokens > 0 {
		results = append([]AggregatedUsage{currentUsage}, results...)
	}

	return results, nil
}

// GetUsageByMonth returns monthly usage for a user
func (db *DB) GetUsageByMonth(userID string) ([]AggregatedUsage, error) {
	now := time.Now()
	currentMonth := now.Format("2006-01")

	var results []AggregatedUsage

	// Get completed months from summary table
	rows, err := db.Query(`
		SELECT period_key, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost
		FROM usage_summary
		WHERE user_id = ? AND period_type = 'month' AND period_key != ?
		ORDER BY period_key DESC
		LIMIT 12
	`, userID, currentMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var u AggregatedUsage
		if err := rows.Scan(&u.Period, &u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens, &u.Cost); err != nil {
			return nil, err
		}
		results = append(results, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get current month's data from raw records
	var currentUsage AggregatedUsage
	currentUsage.Period = currentMonth
	err = db.QueryRow(`
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cost), 0)
		FROM usage_records
		WHERE user_id = ? AND strftime('%Y-%m', timestamp) = ?
	`, userID, currentMonth).Scan(&currentUsage.InputTokens, &currentUsage.OutputTokens, &currentUsage.CacheCreationTokens, &currentUsage.CacheReadTokens, &currentUsage.Cost)
	if err != nil {
		return nil, err
	}

	// Only include current month if there's data
	if currentUsage.InputTokens > 0 || currentUsage.OutputTokens > 0 {
		results = append([]AggregatedUsage{currentUsage}, results...)
	}

	return results, nil
}

// GetTotalUsage returns total usage for a user, optionally filtered by billing period
func (db *DB) GetTotalUsage(userID string, billingDay int) (*AggregatedUsage, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	periodStart, _ := GetBillingPeriod(billingDay)

	var u AggregatedUsage
	u.Period = "Total"

	// Sum completed days from summaries
	summaryQuery := `
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cost), 0)
		FROM usage_summary
		WHERE user_id = ? AND period_type = 'day' AND period_key != ?
	`
	args := []interface{}{userID, today}
	if !periodStart.IsZero() {
		summaryQuery += ` AND period_start >= ?`
		args = append(args, periodStart)
	}

	err := db.QueryRow(summaryQuery, args...).Scan(&u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens, &u.Cost)
	if err != nil {
		return nil, err
	}

	// Add today's data from raw records
	var todayInput, todayOutput, todayCacheCreation, todayCacheRead int64
	var todayCost float64
	err = db.QueryRow(`
		SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cost), 0)
		FROM usage_records
		WHERE user_id = ? AND DATE(timestamp) = ?
	`, userID, today).Scan(&todayInput, &todayOutput, &todayCacheCreation, &todayCacheRead, &todayCost)
	if err != nil {
		return nil, err
	}

	u.InputTokens += todayInput
	u.OutputTokens += todayOutput
	u.CacheCreationTokens += todayCacheCreation
	u.CacheReadTokens += todayCacheRead
	u.Cost += todayCost

	return &u, nil
}

// GetClientSyncStatus returns the last sync time for a client
func (db *DB) GetClientSyncStatus(userID, clientID string) (*time.Time, error) {
	var lastSyncAt sql.NullTime
	err := db.QueryRow(
		`SELECT last_sync_at FROM clients WHERE id = ? AND user_id = ?`,
		clientID, userID,
	).Scan(&lastSyncAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !lastSyncAt.Valid {
		return nil, nil
	}
	return &lastSyncAt.Time, nil
}

// UpdateSummaries updates only the summaries affected by the given records.
// Much more efficient than rebuilding all summaries.
func (db *DB) UpdateSummaries(userID string, billingDay int, records []UsageRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Collect affected periods
	affectedDays := make(map[string]bool)
	affectedMonths := make(map[string]bool)
	affectedCycles := make(map[string]struct{ start, end time.Time })

	for _, r := range records {
		t := r.Timestamp
		dayKey := t.Format("2006-01-02")
		monthKey := t.Format("2006-01")

		affectedDays[dayKey] = true
		affectedMonths[monthKey] = true

		// Calculate affected cycle if billing day is set
		if billingDay > 0 && billingDay <= 31 {
			year, month, dayNum := t.Date()
			var cycleStart time.Time
			clampedDay := clampDay(year, month, billingDay)
			if dayNum >= clampedDay {
				cycleStart = time.Date(year, month, clampedDay, 0, 0, 0, 0, time.Local)
			} else {
				prevMonth := month - 1
				prevYear := year
				if prevMonth < 1 {
					prevMonth = 12
					prevYear--
				}
				cycleStart = time.Date(prevYear, prevMonth, clampDay(prevYear, prevMonth, billingDay), 0, 0, 0, 0, time.Local)
			}

			nextMonth := cycleStart.Month() + 1
			nextYear := cycleStart.Year()
			if nextMonth > 12 {
				nextMonth = 1
				nextYear++
			}
			cycleEnd := time.Date(nextYear, nextMonth, clampDay(nextYear, nextMonth, billingDay), 0, 0, 0, 0, time.Local).Add(-time.Second)
			cycleKey := cycleStart.Format("Jan 2") + " – " + cycleEnd.Format("Jan 2")
			affectedCycles[cycleKey] = struct{ start, end time.Time }{cycleStart, cycleEnd}
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert statement
	stmt, err := tx.Prepare(`
		INSERT INTO usage_summary
		(user_id, period_type, period_key, period_start, period_end, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, period_type, period_key) DO UPDATE SET
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			cache_read_tokens = excluded.cache_read_tokens,
			cost = excluded.cost
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Update day summaries
	for dayKey := range affectedDays {
		dayStart, _ := time.ParseInLocation("2006-01-02", dayKey, time.Local)
		dayEnd := dayStart.Add(24*time.Hour - time.Second)

		var input, output, cacheCreation, cacheRead int64
		var cost float64
		err := tx.QueryRow(`
			SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
			       COALESCE(SUM(cost), 0)
			FROM usage_records
			WHERE user_id = ? AND DATE(timestamp) = ?
		`, userID, dayKey).Scan(&input, &output, &cacheCreation, &cacheRead, &cost)
		if err != nil {
			return err
		}

		if _, err := stmt.Exec(userID, "day", dayKey, dayStart, dayEnd, input, output, cacheCreation, cacheRead, cost); err != nil {
			return err
		}
	}

	// Update month summaries
	for monthKey := range affectedMonths {
		t, _ := time.ParseInLocation("2006-01", monthKey, time.Local)
		monthStart := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
		monthEnd := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.Local).Add(-time.Second)

		var input, output, cacheCreation, cacheRead int64
		var cost float64
		err := tx.QueryRow(`
			SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
			       COALESCE(SUM(cost), 0)
			FROM usage_records
			WHERE user_id = ? AND strftime('%Y-%m', timestamp) = ?
		`, userID, monthKey).Scan(&input, &output, &cacheCreation, &cacheRead, &cost)
		if err != nil {
			return err
		}

		if _, err := stmt.Exec(userID, "month", monthKey, monthStart, monthEnd, input, output, cacheCreation, cacheRead, cost); err != nil {
			return err
		}
	}

	// Update cycle summaries
	for cycleKey, period := range affectedCycles {
		var input, output, cacheCreation, cacheRead int64
		var cost float64
		err := tx.QueryRow(`
			SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
			       COALESCE(SUM(cost), 0)
			FROM usage_records
			WHERE user_id = ? AND timestamp >= ? AND timestamp <= ?
		`, userID, period.start, period.end).Scan(&input, &output, &cacheCreation, &cacheRead, &cost)
		if err != nil {
			return err
		}

		if _, err := stmt.Exec(userID, "cycle", cycleKey, period.start, period.end, input, output, cacheCreation, cacheRead, cost); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RebuildCycleSummaries rebuilds only cycle summaries for a user.
// Use this when billing day changes.
func (db *DB) RebuildCycleSummaries(userID string, billingDay int) error {
	// Clear existing cycle summaries
	if _, err := db.Exec(`DELETE FROM usage_summary WHERE user_id = ? AND period_type = 'cycle'`, userID); err != nil {
		return err
	}

	if billingDay <= 0 || billingDay > 31 {
		return nil
	}

	// Read from day summaries (much faster than raw records)
	rows, err := db.Query(`
		SELECT period_key, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost
		FROM usage_summary
		WHERE user_id = ? AND period_type = 'day'
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	cycles := make(map[string]struct {
		start, end                              time.Time
		input, output, cacheCreation, cacheRead int64
		cost                                    float64
	})

	for rows.Next() {
		var day string
		var input, output, cacheCreation, cacheRead int64
		var cost float64
		if err := rows.Scan(&day, &input, &output, &cacheCreation, &cacheRead, &cost); err != nil {
			return err
		}

		t, _ := time.Parse("2006-01-02", day)
		year, month, dayNum := t.Date()

		var cycleStart time.Time
		clampedDay := clampDay(year, month, billingDay)
		if dayNum >= clampedDay {
			cycleStart = time.Date(year, month, clampedDay, 0, 0, 0, 0, time.Local)
		} else {
			prevMonth := month - 1
			prevYear := year
			if prevMonth < 1 {
				prevMonth = 12
				prevYear--
			}
			cycleStart = time.Date(prevYear, prevMonth, clampDay(prevYear, prevMonth, billingDay), 0, 0, 0, 0, time.Local)
		}

		nextMonth := cycleStart.Month() + 1
		nextYear := cycleStart.Year()
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		cycleEnd := time.Date(nextYear, nextMonth, clampDay(nextYear, nextMonth, billingDay), 0, 0, 0, 0, time.Local).Add(-time.Second)
		cycleKey := cycleStart.Format("Jan 2") + " – " + cycleEnd.Format("Jan 2")

		c := cycles[cycleKey]
		c.start = cycleStart
		c.end = cycleEnd
		c.input += input
		c.output += output
		c.cacheCreation += cacheCreation
		c.cacheRead += cacheRead
		c.cost += cost
		cycles[cycleKey] = c
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Insert cycle summaries
	for cycleKey, c := range cycles {
		_, err := db.Exec(`
			INSERT INTO usage_summary
			(user_id, period_type, period_key, period_start, period_end, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, cost)
			VALUES (?, 'cycle', ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, cycleKey, c.start, c.end, c.input, c.output, c.cacheCreation, c.cacheRead, c.cost)
		if err != nil {
			return err
		}
	}

	return nil
}
