package subscription

import (
	"context"
	"database/sql"
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

type Subscription struct {
	ID            string    `json:"id" db:"id"`
	UserID        string    `json:"user_id" db:"user_id"`
	PlanID        string    `json:"plan_id" db:"plan_id"`
	Status        string    `json:"status" db:"status"`
	StartDate     time.Time `json:"start_date" db:"start_date"`
	EndDate       time.Time `json:"end_date" db:"end_date"`
	AutoRenew     bool      `json:"auto_renew" db:"auto_renew"`
	PaymentMethod string    `json:"payment_method" db:"payment_method"`
	Amount        float64   `json:"amount" db:"amount"`
	Currency      string    `json:"currency" db:"currency"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

type CreateSubscriptionRequest struct {
	UserID        string  `json:"user_id" binding:"required"`
	PlanID        string  `json:"plan_id" binding:"required"`
	PaymentMethod string  `json:"payment_method" binding:"required"`
	Amount        float64 `json:"amount" binding:"required"`
	Currency      string  `json:"currency" binding:"required"`
	AutoRenew     bool    `json:"auto_renew"`
}

type UpdateSubscriptionRequest struct {
	Status        *string  `json:"status"`
	AutoRenew     *bool    `json:"auto_renew"`
	PaymentMethod *string  `json:"payment_method"`
	Amount        *float64 `json:"amount"`
	Currency      *string  `json:"currency"`
}

func NewService(db *db.Connection, cache *cache.RedisClient) *Service {
	return &Service{
		db:    db,
		cache: cache,
	}
}

func (s *Service) CreateSubscription(c *gin.Context) {
	var req CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordSubscriptionOperation("create", "validation_error")
		return
	}

	// Check if user already has an active subscription
	existing, err := s.GetActiveSubscriptionByUserID(c.Request.Context(), req.UserID)
	if err != nil && err != sql.ErrNoRows {
		logrus.Errorf("Failed to check existing subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("create", "db_error")
		return
	}

	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already has an active subscription"})
		telemetry.RecordSubscriptionOperation("create", "conflict")
		return
	}

	// Create subscription
	subscription := &Subscription{
		ID:            generateID(),
		UserID:        req.UserID,
		PlanID:        req.PlanID,
		Status:        "active",
		StartDate:     time.Now(),
		EndDate:       time.Now().AddDate(0, 1, 0), // 1 month
		AutoRenew:     req.AutoRenew,
		PaymentMethod: req.PaymentMethod,
		Amount:        req.Amount,
		Currency:      req.Currency,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.createSubscription(c.Request.Context(), subscription); err != nil {
		logrus.Errorf("Failed to create subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("create", "db_error")
		return
	}

	// Cache the subscription
	s.cacheSubscription(c.Request.Context(), subscription)

	c.JSON(http.StatusCreated, subscription)
	telemetry.RecordSubscriptionOperation("create", "success")
}

func (s *Service) GetSubscription(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription ID is required"})
		return
	}

	// Try cache first
	cached, err := s.getCachedSubscription(c.Request.Context(), id)
	if err == nil && cached != nil {
		c.JSON(http.StatusOK, cached)
		telemetry.RecordSubscriptionOperation("get", "cache_hit")
		return
	}

	// Get from database
	subscription, err := s.getSubscriptionByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
			telemetry.RecordSubscriptionOperation("get", "not_found")
			return
		}
		logrus.Errorf("Failed to get subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("get", "db_error")
		return
	}

	// Cache the subscription
	s.cacheSubscription(c.Request.Context(), subscription)

	c.JSON(http.StatusOK, subscription)
	telemetry.RecordSubscriptionOperation("get", "success")
}

func (s *Service) UpdateSubscription(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription ID is required"})
		return
	}

	var req UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordSubscriptionOperation("update", "validation_error")
		return
	}

	// Get existing subscription
	subscription, err := s.getSubscriptionByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
			telemetry.RecordSubscriptionOperation("update", "not_found")
			return
		}
		logrus.Errorf("Failed to get subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("update", "db_error")
		return
	}

	// Update fields
	if req.Status != nil {
		subscription.Status = *req.Status
	}
	if req.AutoRenew != nil {
		subscription.AutoRenew = *req.AutoRenew
	}
	if req.PaymentMethod != nil {
		subscription.PaymentMethod = *req.PaymentMethod
	}
	if req.Amount != nil {
		subscription.Amount = *req.Amount
	}
	if req.Currency != nil {
		subscription.Currency = *req.Currency
	}

	subscription.UpdatedAt = time.Now()

	// Update in database
	if err := s.updateSubscription(c.Request.Context(), subscription); err != nil {
		logrus.Errorf("Failed to update subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("update", "db_error")
		return
	}

	// Update cache
	s.cacheSubscription(c.Request.Context(), subscription)

	c.JSON(http.StatusOK, subscription)
	telemetry.RecordSubscriptionOperation("update", "success")
}

func (s *Service) CancelSubscription(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription ID is required"})
		return
	}

	// Get existing subscription
	subscription, err := s.getSubscriptionByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
			telemetry.RecordSubscriptionOperation("cancel", "not_found")
			return
		}
		logrus.Errorf("Failed to get subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("cancel", "db_error")
		return
	}

	// Cancel subscription
	subscription.Status = "cancelled"
	subscription.UpdatedAt = time.Now()

	if err := s.updateSubscription(c.Request.Context(), subscription); err != nil {
		logrus.Errorf("Failed to cancel subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("cancel", "db_error")
		return
	}

	// Update cache
	s.cacheSubscription(c.Request.Context(), subscription)

	c.JSON(http.StatusOK, subscription)
	telemetry.RecordSubscriptionOperation("cancel", "success")
}

func (s *Service) RenewSubscription(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription ID is required"})
		return
	}

	// Get existing subscription
	subscription, err := s.getSubscriptionByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
			telemetry.RecordSubscriptionOperation("renew", "not_found")
			return
		}
		logrus.Errorf("Failed to get subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("renew", "db_error")
		return
	}

	// Check if subscription can be renewed
	if subscription.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subscription is not active"})
		telemetry.RecordSubscriptionOperation("renew", "invalid_status")
		return
	}

	// Extend subscription
	subscription.EndDate = subscription.EndDate.AddDate(0, 1, 0) // Add 1 month
	subscription.UpdatedAt = time.Now()

	if err := s.updateSubscription(c.Request.Context(), subscription); err != nil {
		logrus.Errorf("Failed to renew subscription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordSubscriptionOperation("renew", "db_error")
		return
	}

	// Update cache
	s.cacheSubscription(c.Request.Context(), subscription)

	c.JSON(http.StatusOK, subscription)
	telemetry.RecordSubscriptionOperation("renew", "success")
}

// Helper methods
func (s *Service) createSubscription(ctx context.Context, sub *Subscription) error {
	query := `
		INSERT INTO subscriptions (id, user_id, plan_id, status, start_date, end_date, 
			auto_renew, payment_method, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := s.db.ExecContext(ctx, query, sub.ID, sub.UserID, sub.PlanID, sub.Status,
		sub.StartDate, sub.EndDate, sub.AutoRenew, sub.PaymentMethod, sub.Amount,
		sub.Currency, sub.CreatedAt, sub.UpdatedAt)
	return err
}

func (s *Service) getSubscriptionByID(ctx context.Context, id string) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, start_date, end_date, auto_renew,
			payment_method, amount, currency, created_at, updated_at
		FROM subscriptions WHERE id = $1
	`
	var sub Subscription
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartDate, &sub.EndDate,
		&sub.AutoRenew, &sub.PaymentMethod, &sub.Amount, &sub.Currency,
		&sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) GetActiveSubscriptionByUserID(ctx context.Context, userID string) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, start_date, end_date, auto_renew,
			payment_method, amount, currency, created_at, updated_at
		FROM subscriptions 
		WHERE user_id = $1 AND status = 'active' AND end_date > NOW()
		ORDER BY created_at DESC LIMIT 1
	`
	var sub Subscription
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Status, &sub.StartDate, &sub.EndDate,
		&sub.AutoRenew, &sub.PaymentMethod, &sub.Amount, &sub.Currency,
		&sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) updateSubscription(ctx context.Context, sub *Subscription) error {
	query := `
		UPDATE subscriptions 
		SET status = $1, start_date = $2, end_date = $3, auto_renew = $4,
			payment_method = $5, amount = $6, currency = $7, updated_at = $8
		WHERE id = $9
	`
	_, err := s.db.ExecContext(ctx, query, sub.Status, sub.StartDate, sub.EndDate,
		sub.AutoRenew, sub.PaymentMethod, sub.Amount, sub.Currency, sub.UpdatedAt, sub.ID)
	return err
}

func (s *Service) cacheSubscription(ctx context.Context, sub *Subscription) {
	key := fmt.Sprintf("subscription:%s", sub.ID)
	data, err := json.Marshal(sub)
	if err != nil {
		logrus.Errorf("Failed to marshal subscription for cache: %v", err)
		return
	}

	// Cache for 1 hour
	if err := s.cache.Set(ctx, key, string(data), time.Hour); err != nil {
		logrus.Errorf("Failed to cache subscription: %v", err)
	}
}

func (s *Service) getCachedSubscription(ctx context.Context, id string) (*Subscription, error) {
	key := fmt.Sprintf("subscription:%s", id)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var sub Subscription
	if err := json.Unmarshal([]byte(data), &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func generateID() string {
	return fmt.Sprintf("sub_%d", time.Now().UnixNano())
}
