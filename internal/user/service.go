package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"scalable-paywall/internal/cache"
	"scalable-paywall/internal/db"
	"scalable-paywall/internal/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Service struct {
	db    *db.Connection
	cache *cache.RedisClient
}

type User struct {
	ID        string    `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Username  string    `json:"username" db:"username"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required,min=3,max=50"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"`
	Username *string `json:"username,omitempty"`
	Status   *string `json:"status,omitempty"`
}

type UserSession struct {
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewService(db *db.Connection, cache *cache.RedisClient) *Service {
	return &Service{
		db:    db,
		cache: cache,
	}
}

func (s *Service) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordUserOperation("create", "validation_error")
		return
	}

	// Check if user already exists
	existing, err := s.getUserByEmail(c.Request.Context(), req.Email)
	if err == nil && existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User with this email already exists"})
		telemetry.RecordUserOperation("create", "conflict")
		return
	}

	existing, err = s.getUserByUsername(c.Request.Context(), req.Username)
	if err == nil && existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Username already taken"})
		telemetry.RecordUserOperation("create", "conflict")
		return
	}

	// Create user
	user := &User{
		ID:        generateUUID(),
		Email:     req.Email,
		Username:  req.Username,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.createUser(c.Request.Context(), user); err != nil {
		logrus.Errorf("Failed to create user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordUserOperation("create", "db_error")
		return
	}

	// Cache the user
	s.cacheUser(c.Request.Context(), user)

	c.JSON(http.StatusCreated, user)
	telemetry.RecordUserOperation("create", "success")
}

func (s *Service) GetUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	// Try cache first
	cached, err := s.getCachedUser(c.Request.Context(), id)
	if err == nil && cached != nil {
		c.JSON(http.StatusOK, cached)
		telemetry.RecordUserOperation("get", "cache_hit")
		return
	}

	// Get from database
	user, err := s.getUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		telemetry.RecordUserOperation("get", "not_found")
		return
	}

	// Cache the user
	s.cacheUser(c.Request.Context(), user)

	c.JSON(http.StatusOK, user)
	telemetry.RecordUserOperation("get", "success")
}

func (s *Service) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordUserOperation("update", "validation_error")
		return
	}

	// Get existing user
	user, err := s.getUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		telemetry.RecordUserOperation("update", "not_found")
		return
	}

	// Update fields
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Status != nil {
		user.Status = *req.Status
	}

	user.UpdatedAt = time.Now()

	// Update in database
	if err := s.updateUser(c.Request.Context(), user); err != nil {
		logrus.Errorf("Failed to update user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordUserOperation("update", "db_error")
		return
	}

	// Update cache
	s.cacheUser(c.Request.Context(), user)

	c.JSON(http.StatusOK, user)
	telemetry.RecordUserOperation("update", "success")
}

func (s *Service) CreateSession(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify user exists
	user, err := s.getUserByID(c.Request.Context(), req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Generate session token
	token := generateSessionToken()
	session := &UserSession{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hour session
	}

	// Store session in cache
	s.cacheSession(c.Request.Context(), session)

	c.JSON(http.StatusOK, session)
}

func (s *Service) ValidateSession(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token required"})
		return
	}

	// Remove "Bearer " prefix if present
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	// Get session from cache
	session, err := s.getCachedSession(c.Request.Context(), token)
	if err != nil || session == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired session"})
		return
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		s.cache.Delete(c.Request.Context(), fmt.Sprintf("session:%s", token))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session expired"})
		return
	}

	// Set user ID in context for downstream handlers
	c.Set("user_id", session.UserID)
	c.Next()
}

// Helper methods
func (s *Service) createUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (id, email, username, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query, user.ID, user.Email, user.Username,
		user.Status, user.CreatedAt, user.UpdatedAt)
	return err
}

func (s *Service) getUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, username, status, created_at, updated_at
		FROM users WHERE id = $1
	`
	var user User
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Username, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) getUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, username, status, created_at, updated_at
		FROM users WHERE email = $1
	`
	var user User
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Username, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) getUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, email, username, status, created_at, updated_at
		FROM users WHERE username = $1
	`
	var user User
	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) updateUser(ctx context.Context, user *User) error {
	query := `
		UPDATE users 
		SET email = $1, username = $2, status = $3, updated_at = $4
		WHERE id = $5
	`
	_, err := s.db.ExecContext(ctx, query, user.Email, user.Username,
		user.Status, user.UpdatedAt, user.ID)
	return err
}

func (s *Service) cacheUser(ctx context.Context, user *User) {
	key := fmt.Sprintf("user:%s", user.ID)
	data, err := json.Marshal(user)
	if err != nil {
		logrus.Errorf("Failed to marshal user for cache: %v", err)
		return
	}

	// Cache for 1 hour
	if err := s.cache.Set(ctx, key, string(data), time.Hour); err != nil {
		logrus.Errorf("Failed to cache user: %v", err)
	}
}

func (s *Service) getCachedUser(ctx context.Context, id string) (*User, error) {
	key := fmt.Sprintf("user:%s", id)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var user User
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) cacheSession(ctx context.Context, session *UserSession) {
	key := fmt.Sprintf("session:%s", session.Token)
	data, err := json.Marshal(session)
	if err != nil {
		logrus.Errorf("Failed to marshal session for cache: %v", err)
		return
	}

	// Cache until session expires
	ttl := session.ExpiresAt.Sub(time.Now())
	if err := s.cache.Set(ctx, key, string(data), ttl); err != nil {
		logrus.Errorf("Failed to cache session: %v", err)
	}
}

func (s *Service) getCachedSession(ctx context.Context, token string) (*UserSession, error) {
	key := fmt.Sprintf("session:%s", token)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var session UserSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
