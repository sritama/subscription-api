package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"scalable-paywall/internal/cache"
	"scalable-paywall/internal/config"
	"scalable-paywall/internal/db"
	"scalable-paywall/internal/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Service struct {
	cfg            *config.PaymentConfig
	db             *db.Connection
	cache          *cache.RedisClient
	circuitBreaker *CircuitBreaker
}

type CircuitBreaker struct {
	mu              sync.RWMutex
	state           State
	failureCount    int
	lastFailureTime time.Time
	config          config.CircuitBreakerConfig
}

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type PaymentRequest struct {
	UserID        string  `json:"user_id" binding:"required"`
	PlanID        string  `json:"plan_id" binding:"required"`
	Amount        float64 `json:"amount" binding:"required"`
	Currency      string  `json:"currency" binding:"required"`
	PaymentMethod string  `json:"payment_method" binding:"required"`
	Description   string  `json:"description"`
}

type PaymentResponse struct {
	TransactionID string    `json:"transaction_id"`
	Status        string    `json:"status"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`
	GatewayID     string    `json:"gateway_id,omitempty"`
}

type WebhookEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Created   int64                  `json:"created"`
	Processed bool                   `json:"processed"`
}

func NewService(cfg *config.PaymentConfig, db *db.Connection, cache *cache.RedisClient) *Service {
	return &Service{
		cfg:            cfg,
		db:             db,
		cache:          cache,
		circuitBreaker: NewCircuitBreaker(cfg.CircuitBreaker),
	}
}

func (s *Service) ProcessPayment(c *gin.Context) {
	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordPaymentOperation("process", "validation_error")
		return
	}

	// Check circuit breaker
	if !s.circuitBreaker.CanExecute() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service temporarily unavailable"})
		telemetry.RecordPaymentOperation("process", "circuit_breaker_open")
		return
	}

	// Process payment through gateway
	response, err := s.processPaymentThroughGateway(c.Request.Context(), req)
	if err != nil {
		s.circuitBreaker.RecordFailure()
		logrus.Errorf("Payment processing failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment processing failed"})
		telemetry.RecordPaymentOperation("process", "gateway_error")
		return
	}

	// Record success
	s.circuitBreaker.RecordSuccess()

	// Store transaction in database
	if err := s.storeTransaction(c.Request.Context(), req, response); err != nil {
		logrus.Errorf("Failed to store transaction: %v", err)
		// Don't fail the request, just log the error
	}

	c.JSON(http.StatusOK, response)
	telemetry.RecordPaymentOperation("process", "success")
}

func (s *Service) HandleWebhook(c *gin.Context) {
	// Verify webhook signature
	if !s.verifyWebhookSignature(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid webhook signature"})
		return
	}

	var event WebhookEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Store webhook event
	if err := s.storeWebhookEvent(c.Request.Context(), event); err != nil {
		logrus.Errorf("Failed to store webhook event: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process webhook"})
		return
	}

	// Process webhook asynchronously
	go s.processWebhookEvent(context.Background(), event)

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (s *Service) GetTransaction(c *gin.Context) {
	transactionID := c.Param("id")
	if transactionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Transaction ID is required"})
		return
	}

	// Try cache first
	cached, err := s.getCachedTransaction(c.Request.Context(), transactionID)
	if err == nil && cached != nil {
		c.JSON(http.StatusOK, cached)
		return
	}

	// Get from database
	transaction, err := s.getTransactionByID(c.Request.Context(), transactionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	// Cache the transaction
	s.cacheTransaction(c.Request.Context(), transaction)

	c.JSON(http.StatusOK, transaction)
}

// Helper methods
func (s *Service) processPaymentThroughGateway(ctx context.Context, req PaymentRequest) (*PaymentResponse, error) {
	// Simulate payment gateway call
	// In production, this would call Stripe, PayPal, etc.

	// Simulate network delay
	time.Sleep(100 * time.Millisecond)

	// Simulate random failures for testing circuit breaker
	if time.Now().UnixNano()%100 < 5 { // 5% failure rate
		return nil, fmt.Errorf("gateway timeout")
	}

	response := &PaymentResponse{
		TransactionID: fmt.Sprintf("txn_%d", time.Now().UnixNano()),
		Status:        "completed",
		Amount:        req.Amount,
		Currency:      req.Currency,
		CreatedAt:     time.Now(),
		GatewayID:     fmt.Sprintf("gw_%d", time.Now().UnixNano()),
	}

	return response, nil
}

func (s *Service) storeTransaction(ctx context.Context, req PaymentRequest, response *PaymentResponse) error {
	query := `
		INSERT INTO payment_transactions (id, user_id, amount, currency, status, 
			payment_method, gateway_transaction_id, gateway_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	gatewayResponse, _ := json.Marshal(map[string]interface{}{
		"status":     response.Status,
		"gateway_id": response.GatewayID,
	})

	_, err := s.db.ExecContext(ctx, query, response.TransactionID, req.UserID,
		response.Amount, response.Currency, response.Status, req.PaymentMethod,
		response.GatewayID, string(gatewayResponse))

	return err
}

func (s *Service) storeWebhookEvent(ctx context.Context, event WebhookEvent) error {
	query := `
		INSERT INTO webhook_events (id, event_type, source, payload)
		VALUES ($1, $2, $3, $4)
	`

	payload, _ := json.Marshal(event)

	_, err := s.db.ExecContext(ctx, query, event.ID, event.Type, "stripe", string(payload))
	return err
}

func (s *Service) processWebhookEvent(ctx context.Context, event WebhookEvent) {
	// Process different webhook event types
	switch event.Type {
	case "payment_intent.succeeded":
		s.handlePaymentSuccess(ctx, event)
	case "payment_intent.payment_failed":
		s.handlePaymentFailure(ctx, event)
	case "invoice.payment_succeeded":
		s.handleInvoicePayment(ctx, event)
	default:
		logrus.Infof("Unhandled webhook event type: %s", event.Type)
	}

	// Mark as processed
	s.markWebhookProcessed(ctx, event.ID)
}

func (s *Service) handlePaymentSuccess(ctx context.Context, event WebhookEvent) {
	logrus.Infof("Processing payment success webhook: %s", event.ID)
	// Update subscription status, send confirmation email, etc.
}

func (s *Service) handlePaymentFailure(ctx context.Context, event WebhookEvent) {
	logrus.Infof("Processing payment failure webhook: %s", event.ID)
	// Update subscription status, send failure notification, etc.
}

func (s *Service) handleInvoicePayment(ctx context.Context, event WebhookEvent) {
	logrus.Infof("Processing invoice payment webhook: %s", event.ID)
	// Handle recurring payment, update subscription dates, etc.
}

func (s *Service) markWebhookProcessed(ctx context.Context, eventID string) error {
	query := `UPDATE webhook_events SET processed = true, processed_at = NOW() WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, eventID)
	return err
}

func (s *Service) verifyWebhookSignature(c *gin.Context) bool {
	// In production, verify webhook signature using the webhook secret
	// For now, just return true for development
	return true
}

func (s *Service) getTransactionByID(ctx context.Context, id string) (map[string]interface{}, error) {
	// Implementation would query the database
	// For now, return a mock response
	transaction := map[string]interface{}{
		"id":         id,
		"status":     "completed",
		"created_at": time.Now(),
	}

	return transaction, nil
}

func (s *Service) cacheTransaction(ctx context.Context, transaction map[string]interface{}) {
	key := fmt.Sprintf("transaction:%s", transaction["id"])
	data, _ := json.Marshal(transaction)
	s.cache.Set(ctx, key, string(data), time.Hour)
}

func (s *Service) getCachedTransaction(ctx context.Context, id string) (map[string]interface{}, error) {
	key := fmt.Sprintf("transaction:%s", id)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var transaction map[string]interface{}
	if err := json.Unmarshal([]byte(data), &transaction); err != nil {
		return nil, err
	}
	return transaction, nil
}

// Circuit Breaker Implementation
func NewCircuitBreaker(cfg config.CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  Closed,
		config: cfg,
	}
}

func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case Closed:
		return true
	case Open:
		if time.Since(cb.lastFailureTime) > time.Duration(cb.config.RecoveryTimeout)*time.Second {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = HalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case HalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = Closed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.config.FailureThreshold {
		cb.state = Open
	}
}
