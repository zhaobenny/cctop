package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/zhaobenny/cctop/server/internal/auth"
	"github.com/zhaobenny/cctop/server/internal/database"
	"github.com/zhaobenny/cctop/server/internal/handlers"
	"github.com/zhaobenny/cctop/server/internal/middleware"
	"github.com/zhaobenny/cctop/server/internal/templates"
)

var version = "dev"

func main() {
	// Load configuration from environment
	port := getEnv("PORT", "8080")
	dbPath := getDBPath()

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	// Open database
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Setup session manager with SQLite store
	sessionMgr := scs.New()
	sessionMgr.Store = sqlite3store.New(db.DB)
	sessionMgr.Lifetime = 7 * 24 * time.Hour
	sessionMgr.Cookie.Secure = isProduction()
	sessionMgr.Cookie.SameSite = http.SameSiteLaxMode

	// Setup rate limiter for auth endpoints (5 requests per minute, burst of 5)
	authLimiter := middleware.NewIPRateLimiter(5.0/60.0, 5)

	// Parse templates
	tmpl, err := templates.Parse()
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Create handlers
	h := handlers.New(db, sessionMgr, tmpl)
	authMiddleware := auth.NewMiddleware(db, sessionMgr)

	// Setup routes
	mux := http.NewServeMux()

	// Health check (for orchestrators)
	mux.HandleFunc("/health", h.Health)

	// Static files (embedded)
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Public routes
	mux.HandleFunc("/", h.Index)
	mux.HandleFunc("/partial/auth", h.PartialAuth)
	mux.Handle("/login", authLimiter.LimitFunc(h.Login))
	mux.Handle("/register", authLimiter.LimitFunc(h.Register))

	// Protected routes (session-based)
	mux.Handle("/logout", authMiddleware.RequireAuth(http.HandlerFunc(h.Logout)))
	mux.Handle("/partial/dashboard", authMiddleware.RequireAuth(http.HandlerFunc(h.PartialDashboard)))
	mux.Handle("/partial/usage-table", authMiddleware.RequireAuth(http.HandlerFunc(h.PartialUsageTable)))
	mux.Handle("/settings/billing-day", authMiddleware.RequireAuth(http.HandlerFunc(h.UpdateBillingDay)))

	// API routes (API key-based)
	mux.Handle("/api/sync", authMiddleware.RequireAPIKey(http.HandlerFunc(h.APISync)))
	mux.Handle("/api/sync/status", authMiddleware.RequireAPIKey(http.HandlerFunc(h.APISyncStatus)))

	// Wrap with session middleware and security headers
	handler := middleware.SecurityHeaders(sessionMgr.LoadAndSave(mux))

	// Start server
	addr := ":" + port
	log.Printf("Starting cctop-server %s on %s", version, addr)
	log.Printf("Database: %s", dbPath)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDBPath() string {
	// Env var takes precedence (for Docker, custom deployments)
	if path := os.Getenv("DB_PATH"); path != "" {
		return path
	}

	// Fall back to user config dir
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "./cctop.db"
	}

	return filepath.Join(configDir, "cctop-server", "cctop.db")
}

func isProduction() bool {
	env := strings.ToLower(os.Getenv("ENV"))
	return env == "production" || env == "prod"
}
