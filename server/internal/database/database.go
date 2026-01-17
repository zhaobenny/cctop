package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
		 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var inserted int64
	for _, r := range records {
		result, err := stmt.Exec(
			r.UserID, r.ClientID, r.Timestamp, r.SessionID, r.ProjectPath, r.Model,
			r.InputTokens, r.OutputTokens, r.CacheCreationTokens, r.CacheReadTokens,
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
	query := `
		SELECT
			DATE(timestamp) as period,
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens,
			SUM(cache_creation_tokens) as cache_creation_tokens,
			SUM(cache_read_tokens) as cache_read_tokens
		FROM usage_records
		WHERE user_id = ?
	`
	args := []interface{}{userID}

	periodStart, _ := GetBillingPeriod(billingDay)
	if !periodStart.IsZero() {
		query += ` AND DATE(timestamp) >= ?`
		args = append(args, periodStart.Format("2006-01-02"))
	}

	query += ` GROUP BY DATE(timestamp) ORDER BY period DESC LIMIT 30`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AggregatedUsage
	for rows.Next() {
		var u AggregatedUsage
		err := rows.Scan(&u.Period, &u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens)
		if err != nil {
			return nil, err
		}
		u.Cost = calculateCost(u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens)
		results = append(results, u)
	}

	return results, rows.Err()
}

// GetUsageByBillingCycle returns usage grouped by billing cycles
func (db *DB) GetUsageByBillingCycle(userID string, billingDay int) ([]AggregatedUsage, error) {
	if billingDay <= 0 || billingDay > 31 {
		return nil, nil
	}

	// Get all usage records and group them by billing cycle in Go
	// This is simpler than complex SQL for billing cycle boundaries
	query := `
		SELECT
			DATE(timestamp) as day,
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens,
			SUM(cache_creation_tokens) as cache_creation_tokens,
			SUM(cache_read_tokens) as cache_read_tokens
		FROM usage_records
		WHERE user_id = ?
		GROUP BY DATE(timestamp)
		ORDER BY day ASC
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Map to accumulate usage per billing cycle
	cycles := make(map[string]*AggregatedUsage)
	var cycleOrder []string

	for rows.Next() {
		var day string
		var input, output, cacheCreate, cacheRead int64
		if err := rows.Scan(&day, &input, &output, &cacheCreate, &cacheRead); err != nil {
			return nil, err
		}

		// Parse the day and find which billing cycle it belongs to
		t, _ := time.Parse("2006-01-02", day)
		year, month, dayNum := t.Date()

		// Determine billing cycle start
		var cycleStart time.Time
		clampedDay := clampDay(year, month, billingDay)
		if dayNum >= clampedDay {
			cycleStart = time.Date(year, month, clampedDay, 0, 0, 0, 0, t.Location())
		} else {
			prevMonth := month - 1
			prevYear := year
			if prevMonth < 1 {
				prevMonth = 12
				prevYear--
			}
			clampedDay = clampDay(prevYear, prevMonth, billingDay)
			cycleStart = time.Date(prevYear, prevMonth, clampedDay, 0, 0, 0, 0, t.Location())
		}

		// Calculate cycle end
		nextMonth := cycleStart.Month() + 1
		nextYear := cycleStart.Year()
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		cycleEndDay := clampDay(nextYear, nextMonth, billingDay)
		cycleEnd := time.Date(nextYear, nextMonth, cycleEndDay, 0, 0, 0, 0, t.Location()).Add(-time.Second)

		// Format as "Jan 17 – Feb 16"
		cycleKey := cycleStart.Format("Jan 2") + " – " + cycleEnd.Format("Jan 2")

		if _, exists := cycles[cycleKey]; !exists {
			cycles[cycleKey] = &AggregatedUsage{Period: cycleKey}
			cycleOrder = append(cycleOrder, cycleKey)
		}

		cycles[cycleKey].InputTokens += input
		cycles[cycleKey].OutputTokens += output
		cycles[cycleKey].CacheCreationTokens += cacheCreate
		cycles[cycleKey].CacheReadTokens += cacheRead
	}

	// Build result in reverse order (newest first)
	var results []AggregatedUsage
	for i := len(cycleOrder) - 1; i >= 0; i-- {
		u := cycles[cycleOrder[i]]
		u.Cost = calculateCost(u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens)
		results = append(results, *u)
	}

	return results, rows.Err()
}

// GetUsageByMonth returns monthly usage for a user
func (db *DB) GetUsageByMonth(userID string) ([]AggregatedUsage, error) {
	query := `
		SELECT
			strftime('%Y-%m', timestamp) as period,
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens,
			SUM(cache_creation_tokens) as cache_creation_tokens,
			SUM(cache_read_tokens) as cache_read_tokens
		FROM usage_records
		WHERE user_id = ?
		GROUP BY strftime('%Y-%m', timestamp)
		ORDER BY period DESC
		LIMIT 12
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AggregatedUsage
	for rows.Next() {
		var u AggregatedUsage
		err := rows.Scan(&u.Period, &u.InputTokens, &u.OutputTokens, &u.CacheCreationTokens, &u.CacheReadTokens)
		if err != nil {
			return nil, err
		}
		u.Cost = calculateCost(u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens)
		results = append(results, u)
	}

	return results, rows.Err()
}

// GetTotalUsage returns total usage for a user, optionally filtered by billing period
func (db *DB) GetTotalUsage(userID string, billingDay int) (*AggregatedUsage, error) {
	query := `
		SELECT
			SUM(input_tokens) as input_tokens,
			SUM(output_tokens) as output_tokens,
			SUM(cache_creation_tokens) as cache_creation_tokens,
			SUM(cache_read_tokens) as cache_read_tokens
		FROM usage_records
		WHERE user_id = ?
	`
	args := []interface{}{userID}

	periodStart, _ := GetBillingPeriod(billingDay)
	if !periodStart.IsZero() {
		query += ` AND DATE(timestamp) >= ?`
		args = append(args, periodStart.Format("2006-01-02"))
	}

	var u AggregatedUsage
	var inputTokens, outputTokens, cacheCreation, cacheRead sql.NullInt64
	err := db.QueryRow(query, args...).Scan(&inputTokens, &outputTokens, &cacheCreation, &cacheRead)
	if err != nil {
		return nil, err
	}

	u.Period = "Total"
	u.InputTokens = inputTokens.Int64
	u.OutputTokens = outputTokens.Int64
	u.CacheCreationTokens = cacheCreation.Int64
	u.CacheReadTokens = cacheRead.Int64
	u.Cost = calculateCost(u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens)

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

// calculateCost estimates cost using default Sonnet pricing
func calculateCost(input, output, cacheCreation, cacheRead int64) float64 {
	const (
		inputCost         = 3e-06
		outputCost        = 1.5e-05
		cacheCreationCost = 3.75e-06
		cacheReadCost     = 3e-07
	)

	cost := float64(input) * inputCost
	cost += float64(output) * outputCost
	cost += float64(cacheCreation) * cacheCreationCost
	cost += float64(cacheRead) * cacheReadCost
	return cost
}
