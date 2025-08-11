package paywall

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"scalable-paywall/internal/cache"
	"scalable-paywall/internal/subscription"
	"scalable-paywall/internal/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Service struct {
	cache           *cache.RedisClient
	subscriptionSvc *subscription.Service
}

type PaywallCheckRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	ContentID string `json:"content_id" binding:"required"`
	PlanID    string `json:"plan_id" binding:"required"`
}

type PaywallCheckResponse struct {
	HasAccess bool      `json:"has_access"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type PaywallEnforceRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	ContentID string `json:"content_id" binding:"required"`
	Action    string `json:"action" binding:"required"` // "view", "download", "share"
}

type PaywallEnforceResponse struct {
	Allowed   bool      `json:"allowed"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Usage     UsageInfo `json:"usage,omitempty"`
}

type UsageInfo struct {
	Current   int `json:"current"`
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

func NewService(cache *cache.RedisClient, subscriptionSvc *subscription.Service) *Service {
	return &Service{
		cache:           cache,
		subscriptionSvc: subscriptionSvc,
	}
}

func (s *Service) CheckAccess(c *gin.Context) {
	var req PaywallCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordPaywallCheck("validation_error")
		return
	}

	// Try cache first
	cacheKey := fmt.Sprintf("paywall:access:%s:%s:%s", req.UserID, req.ContentID, req.PlanID)
	cached, err := s.getCachedAccess(c.Request.Context(), cacheKey)
	if err == nil && cached != nil {
		c.JSON(http.StatusOK, cached)
		telemetry.RecordPaywallCheck("cache_hit")
		return
	}

	// Check subscription status
	hasAccess, reason, expiresAt, err := s.checkSubscriptionAccess(c.Request.Context(), req.UserID, req.PlanID)
	if err != nil {
		logrus.Errorf("Failed to check subscription access: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPaywallCheck("error")
		return
	}

	response := &PaywallCheckResponse{
		HasAccess: hasAccess,
		Reason:    reason,
		ExpiresAt: expiresAt,
	}

	// Cache the result for 5 minutes
	s.cacheAccessResult(c.Request.Context(), cacheKey, response)

	c.JSON(http.StatusOK, response)
	if hasAccess {
		telemetry.RecordPaywallCheck("access_granted")
	} else {
		telemetry.RecordPaywallCheck("access_denied")
	}
}

func (s *Service) EnforcePaywall(c *gin.Context) {
	var req PaywallEnforceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check rate limiting
	if !s.checkRateLimit(c.Request.Context(), req.UserID, req.Action) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
		return
	}

	// Check subscription access
	hasAccess, reason, expiresAt, err := s.checkSubscriptionAccess(c.Request.Context(), req.UserID, "")
	if err != nil {
		logrus.Errorf("Failed to check subscription access: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied", "reason": reason})
		return
	}

	// Check usage limits
	usage, err := s.checkUsageLimits(c.Request.Context(), req.UserID, req.Action)
	if err != nil {
		logrus.Errorf("Failed to check usage limits: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Increment usage
	if err := s.incrementUsage(c.Request.Context(), req.UserID, req.Action); err != nil {
		logrus.Errorf("Failed to increment usage: %v", err)
		// Don't fail the request, just log the error
	}

	response := &PaywallEnforceResponse{
		Allowed:   true,
		ExpiresAt: expiresAt,
		Usage:     usage,
	}

	c.JSON(http.StatusOK, response)
}

// Helper methods
func (s *Service) checkSubscriptionAccess(ctx context.Context, userID, planID string) (bool, string, time.Time, error) {
	// Get active subscription for user
	sub, err := s.subscriptionSvc.GetActiveSubscriptionByUserID(ctx, userID)
	if err != nil {
		return false, "No active subscription found", time.Time{}, nil
	}

	// Check if subscription is active
	if sub.Status != "active" {
		return false, "Subscription is not active", time.Time{}, nil
	}

	// Check if subscription has expired
	if time.Now().After(sub.EndDate) {
		return false, "Subscription has expired", time.Time{}, nil
	}

	// If planID is specified, check if it matches
	if planID != "" && sub.PlanID != planID {
		return false, "Plan mismatch", time.Time{}, nil
	}

	return true, "Valid subscription", sub.EndDate, nil
}

func (s *Service) checkRateLimit(ctx context.Context, userID, action string) bool {
	key := fmt.Sprintf("rate_limit:%s:%s", userID, action)

	// Get current count
	current, err := s.cache.Get(ctx, key)
	if err != nil {
		// Key doesn't exist, start counting
		s.cache.Set(ctx, key, "1", time.Minute)
		return true
	}

	count, err := strconv.Atoi(current)
	if err != nil {
		// Invalid count, reset
		s.cache.Set(ctx, key, "1", time.Minute)
		return true
	}

	// Check if limit exceeded (10 requests per minute per action)
	if count >= 10 {
		return false
	}

	// Increment count
	s.cache.Incr(ctx, key)
	return true
}

func (s *Service) checkUsageLimits(ctx context.Context, userID, action string) (UsageInfo, error) {
	key := fmt.Sprintf("usage:%s:%s", userID, action)

	// Get current usage
	current, err := s.cache.Get(ctx, key)
	if err != nil {
		// No usage recorded yet
		return UsageInfo{Current: 0, Limit: 100, Remaining: 100}, nil
	}

	count, err := strconv.Atoi(current)
	if err != nil {
		return UsageInfo{Current: 0, Limit: 100, Remaining: 100}, nil
	}

	limit := 100 // Default limit per day
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return UsageInfo{
		Current:   count,
		Limit:     limit,
		Remaining: remaining,
	}, nil
}

func (s *Service) incrementUsage(ctx context.Context, userID, action string) error {
	key := fmt.Sprintf("usage:%s:%s", userID, action)

	// Increment usage counter
	_, err := s.cache.Incr(ctx, key)
	if err != nil {
		return err
	}

	// Set expiration to end of day
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	ttl := endOfDay.Sub(now)

	_, err = s.cache.Expire(ctx, key, ttl)
	return err
}

func (s *Service) cacheAccessResult(ctx context.Context, key string, result *PaywallCheckResponse) {
	data, err := json.Marshal(result)
	if err != nil {
		logrus.Errorf("Failed to marshal paywall result for cache: %v", err)
		return
	}

	// Cache for 5 minutes
	if err := s.cache.Set(ctx, key, string(data), 5*time.Minute); err != nil {
		logrus.Errorf("Failed to cache paywall result: %v", err)
	}
}

func (s *Service) getCachedAccess(ctx context.Context, key string) (*PaywallCheckResponse, error) {
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var result PaywallCheckResponse
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
