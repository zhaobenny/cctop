package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/zhaobenny/cctop/server/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	userIDKey contextKey = "userID"
	userKey   contextKey = "user"
)

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "cctop_" + hex.EncodeToString(bytes), nil
}

// GenerateID generates a random UUID-like ID
func GenerateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Middleware handles session-based authentication
type Middleware struct {
	db         *database.DB
	sessionMgr *scs.SessionManager
}

// NewMiddleware creates a new auth middleware
func NewMiddleware(db *database.DB, sessionMgr *scs.SessionManager) *Middleware {
	return &Middleware{
		db:         db,
		sessionMgr: sessionMgr,
	}
}

// RequireAuth middleware requires a valid session
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := m.sessionMgr.GetString(r.Context(), "userID")
		if userID == "" {
			// For HTMX requests, return the auth fragment
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		user, err := m.db.GetUserByID(userID)
		if err != nil || user == nil {
			m.sessionMgr.Destroy(r.Context())
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		ctx = context.WithValue(ctx, userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAPIKey middleware requires a valid API key
func (m *Middleware) RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Try Authorization: Bearer token
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		user, err := m.db.GetUserByAPIKey(apiKey)
		if err != nil || user == nil {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, user.ID)
		ctx = context.WithValue(ctx, userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID returns the user ID from context
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// GetUser returns the user from context
func GetUser(ctx context.Context) *database.User {
	if user, ok := ctx.Value(userKey).(*database.User); ok {
		return user
	}
	return nil
}
