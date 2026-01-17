package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/zhaobenny/cctop/server/internal/auth"
	"github.com/zhaobenny/cctop/server/internal/database"
	"github.com/zhaobenny/cctop/server/internal/handlers"
	"github.com/zhaobenny/cctop/server/internal/templates"
)

func main() {
	// Load configuration from environment
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "./cctop.db")

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
	sessionMgr.Cookie.Secure = false // Set to true in production with HTTPS
	sessionMgr.Cookie.SameSite = http.SameSiteLaxMode

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

	// Public routes
	mux.HandleFunc("/", h.Index)
	mux.HandleFunc("/partial/auth", h.PartialAuth)
	mux.HandleFunc("/login", h.Login)
	mux.HandleFunc("/register", h.Register)

	// Protected routes (session-based)
	mux.Handle("/logout", authMiddleware.RequireAuth(http.HandlerFunc(h.Logout)))
	mux.Handle("/partial/dashboard", authMiddleware.RequireAuth(http.HandlerFunc(h.PartialDashboard)))
	mux.Handle("/partial/usage-table", authMiddleware.RequireAuth(http.HandlerFunc(h.PartialUsageTable)))
	mux.Handle("/settings/reset-date", authMiddleware.RequireAuth(http.HandlerFunc(h.UpdateResetDate)))

	// API routes (API key-based)
	mux.Handle("/api/sync", authMiddleware.RequireAPIKey(http.HandlerFunc(h.APISync)))
	mux.Handle("/api/sync/status", authMiddleware.RequireAPIKey(http.HandlerFunc(h.APISyncStatus)))

	// Wrap with session middleware
	handler := sessionMgr.LoadAndSave(mux)

	// Start server
	addr := ":" + port
	log.Printf("Starting cctop-server on %s", addr)
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
