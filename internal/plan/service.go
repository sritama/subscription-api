package plan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"scalable-paywall/internal/cache"
	"scalable-paywall/internal/db"
	"scalable-paywall/internal/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Custom error types
var (
	ErrPlanNotFound         = errors.New("plan not found")
	ErrPlanNameExists       = errors.New("plan with this name already exists")
	ErrPlanHasSubscriptions = errors.New("cannot delete plan with active subscriptions")
	ErrInvalidPlanData      = errors.New("invalid plan data")
)

// Response structures
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Service struct {
	db        *db.Connection
	cache     *cache.RedisClient
	validator *validator.Validate
}

type Plan struct {
	ID               string                 `json:"id" db:"id"`
	Name             string                 `json:"name" db:"name"`
	Description      *string                `json:"description" db:"description"`
	Price            float64                `json:"price" db:"price"`
	Currency         string                 `json:"currency" db:"currency"`
	BillingCycle     string                 `json:"billing_cycle" db:"billing_cycle"`
	Features         map[string]interface{} `json:"features" db:"features"`
	MaxUsagePerDay   *int                   `json:"max_usage_per_day" db:"max_usage_per_day"`
	MaxUsagePerMonth *int                   `json:"max_usage_per_month" db:"max_usage_per_month"`
	IsActive         bool                   `json:"is_active" db:"is_active"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
}

type CreatePlanRequest struct {
	Name             string                 `json:"name" validate:"required"`
	Description      *string                `json:"description"`
	Price            float64                `json:"price" validate:"required,min=0"`
	Currency         string                 `json:"currency" validate:"required,len=3"`
	BillingCycle     string                 `json:"billing_cycle" validate:"required,oneof=monthly yearly weekly daily"`
	Features         map[string]interface{} `json:"features"`
	MaxUsagePerDay   *int                   `json:"max_usage_per_day" validate:"omitempty,min=0"`
	MaxUsagePerMonth *int                   `json:"max_usage_per_month" validate:"omitempty,min=0"`
	IsActive         *bool                  `json:"is_active"`
}

type UpdatePlanRequest struct {
	Name             *string                 `json:"name" validate:"omitempty"`
	Description      *string                 `json:"description"`
	Price            *float64                `json:"price" validate:"omitempty,min=0"`
	Currency         *string                 `json:"currency" validate:"omitempty,len=3"`
	BillingCycle     *string                 `json:"billing_cycle" validate:"omitempty,oneof=monthly yearly weekly daily"`
	Features         *map[string]interface{} `json:"features"`
	MaxUsagePerDay   *int                    `json:"max_usage_per_day" validate:"omitempty,min=0"`
	MaxUsagePerMonth *int                    `json:"max_usage_per_month" validate:"omitempty,min=0"`
	IsActive         *bool                   `json:"is_active"`
}

type PlanListResponse struct {
	Plans []Plan `json:"plans"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

// Plan comparison structures
type PlanComparison struct {
	Plans   []PlanComparisonItem  `json:"plans"`
	Summary PlanComparisonSummary `json:"summary"`
}

type PlanComparisonItem struct {
	Plan           Plan                   `json:"plan"`
	FeatureMatrix  map[string]interface{} `json:"feature_matrix"`
	PriceAnalysis  PriceAnalysis          `json:"price_analysis"`
	Recommendation string                 `json:"recommendation,omitempty"`
}

type PriceAnalysis struct {
	MonthlyCost    float64  `json:"monthly_cost"`
	YearlyCost     float64  `json:"yearly_cost"`
	CostPerFeature float64  `json:"cost_per_feature,omitempty"`
	Savings        *float64 `json:"savings,omitempty"`
}

type PlanComparisonSummary struct {
	CheapestPlan      string  `json:"cheapest_plan"`
	MostExpensivePlan string  `json:"most_expensive_plan"`
	BestValuePlan     string  `json:"best_value_plan,omitempty"`
	PriceRange        float64 `json:"price_range"`
}

// Plan analytics structures
type PlanAnalytics struct {
	PlanID          string             `json:"plan_id"`
	PlanName        string             `json:"plan_name"`
	UsageStats      UsageStatistics    `json:"usage_stats"`
	Popularity      PopularityMetrics  `json:"popularity"`
	Performance     PerformanceMetrics `json:"performance"`
	Recommendations []string           `json:"recommendations"`
}

type UsageStatistics struct {
	TotalSubscriptions   int     `json:"total_subscriptions"`
	ActiveSubscriptions  int     `json:"active_subscriptions"`
	AverageUsagePerDay   float64 `json:"average_usage_per_day"`
	AverageUsagePerMonth float64 `json:"average_usage_per_month"`
	PeakUsageDay         string  `json:"peak_usage_day,omitempty"`
	PeakUsageMonth       string  `json:"peak_usage_month,omitempty"`
}

type PopularityMetrics struct {
	SubscriptionGrowth float64 `json:"subscription_growth"`
	RetentionRate      float64 `json:"retention_rate"`
	ChurnRate          float64 `json:"churn_rate"`
	PopularityRank     int     `json:"popularity_rank"`
	Trending           string  `json:"trending"` // "up", "down", "stable"
}

type PerformanceMetrics struct {
	RevenueGenerated     float64 `json:"revenue_generated"`
	AverageLifetime      float64 `json:"average_lifetime_days"`
	ConversionRate       float64 `json:"conversion_rate"`
	CustomerSatisfaction float64 `json:"customer_satisfaction,omitempty"`
}

func NewService(db *db.Connection, cache *cache.RedisClient) *Service {
	return &Service{
		db:        db,
		cache:     cache,
		validator: validator.New(),
	}
}

func (s *Service) CreatePlan(c *gin.Context) {
	var req CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request format",
			Code:    "INVALID_JSON",
			Details: err.Error(),
		})
		telemetry.RecordPlanOperation("create", "validation_error")
		return
	}

	// Validate request using custom validator
	if err := s.validatePlanRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Validation failed",
			Code:    "VALIDATION_ERROR",
			Details: err.Error(),
		})
		telemetry.RecordPlanOperation("create", "validation_error")
		return
	}

	// Check if plan name already exists
	exists, err := s.planNameExists(c.Request.Context(), req.Name)
	if err != nil {
		logrus.Errorf("Failed to check plan name existence: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Internal server error",
			Code:  "DB_ERROR",
		})
		telemetry.RecordPlanOperation("create", "db_error")
		return
	}

	if exists {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "Plan with this name already exists",
			Code:    "PLAN_NAME_EXISTS",
			Details: "A plan with this name already exists in the system",
		})
		telemetry.RecordPlanOperation("create", "conflict")
		return
	}

	// Set default values
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	// Create plan
	plan := &Plan{
		ID:               generateID(),
		Name:             req.Name,
		Description:      req.Description,
		Price:            req.Price,
		Currency:         req.Currency,
		BillingCycle:     req.BillingCycle,
		Features:         req.Features,
		MaxUsagePerDay:   req.MaxUsagePerDay,
		MaxUsagePerMonth: req.MaxUsagePerMonth,
		IsActive:         isActive,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := s.createPlan(c.Request.Context(), plan); err != nil {
		logrus.Errorf("Failed to create plan: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Internal server error",
			Code:  "DB_ERROR",
		})
		telemetry.RecordPlanOperation("create", "db_error")
		return
	}

	// Cache the plan
	s.cachePlan(c.Request.Context(), plan)

	c.JSON(http.StatusCreated, SuccessResponse{
		Message: "Plan created successfully",
		Data:    plan,
	})
	telemetry.RecordPlanOperation("create", "success")
}

func (s *Service) GetPlan(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plan ID is required"})
		return
	}

	// Try cache first
	cached, err := s.getCachedPlan(c.Request.Context(), id)
	if err == nil && cached != nil {
		c.JSON(http.StatusOK, cached)
		telemetry.RecordPlanOperation("get", "cache_hit")
		return
	}

	// Get from database
	plan, err := s.getPlanByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
			telemetry.RecordPlanOperation("get", "not_found")
			return
		}
		logrus.Errorf("Failed to get plan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("get", "db_error")
		return
	}

	// Cache the plan
	s.cachePlan(c.Request.Context(), plan)

	c.JSON(http.StatusOK, plan)
	telemetry.RecordPlanOperation("get", "success")
}

func (s *Service) UpdatePlan(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plan ID is required"})
		return
	}

	var req UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		telemetry.RecordPlanOperation("update", "validation_error")
		return
	}

	// Get existing plan
	plan, err := s.getPlanByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
			telemetry.RecordPlanOperation("update", "not_found")
			return
		}
		logrus.Errorf("Failed to get plan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("update", "db_error")
		return
	}

	// Check name uniqueness if name is being updated
	if req.Name != nil && *req.Name != plan.Name {
		exists, err := s.planNameExists(c.Request.Context(), *req.Name)
		if err != nil {
			logrus.Errorf("Failed to check plan name existence: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			telemetry.RecordPlanOperation("update", "db_error")
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Plan with this name already exists"})
			telemetry.RecordPlanOperation("update", "conflict")
			return
		}
	}

	// Update fields
	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Description != nil {
		plan.Description = req.Description
	}
	if req.Price != nil {
		plan.Price = *req.Price
	}
	if req.Currency != nil {
		plan.Currency = *req.Currency
	}
	if req.BillingCycle != nil {
		plan.BillingCycle = *req.BillingCycle
	}
	if req.Features != nil {
		plan.Features = *req.Features
	}
	if req.MaxUsagePerDay != nil {
		plan.MaxUsagePerDay = req.MaxUsagePerDay
	}
	if req.MaxUsagePerMonth != nil {
		plan.MaxUsagePerMonth = req.MaxUsagePerMonth
	}
	if req.IsActive != nil {
		plan.IsActive = *req.IsActive
	}

	plan.UpdatedAt = time.Now()

	// Update in database
	if err := s.updatePlan(c.Request.Context(), plan); err != nil {
		logrus.Errorf("Failed to update plan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("update", "db_error")
		return
	}

	// Update cache
	s.cachePlan(c.Request.Context(), plan)

	c.JSON(http.StatusOK, plan)
	telemetry.RecordPlanOperation("update", "success")
}

func (s *Service) DeletePlan(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Plan ID is required"})
		return
	}

	// Check if plan has active subscriptions
	hasSubscriptions, err := s.planHasActiveSubscriptions(c.Request.Context(), id)
	if err != nil {
		logrus.Errorf("Failed to check plan subscriptions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("delete", "db_error")
		return
	}

	if hasSubscriptions {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete plan with active subscriptions"})
		telemetry.RecordPlanOperation("delete", "conflict")
		return
	}

	// Delete plan
	if err := s.deletePlan(c.Request.Context(), id); err != nil {
		logrus.Errorf("Failed to delete plan: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("delete", "db_error")
		return
	}

	// Remove from cache
	s.removeCachedPlan(c.Request.Context(), id)

	c.JSON(http.StatusOK, gin.H{"message": "Plan deleted successfully"})
	telemetry.RecordPlanOperation("delete", "success")
}

func (s *Service) ListPlans(c *gin.Context) {
	// Parse query parameters
	page := 1
	limit := 20
	activeOnly := false

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := parseInt(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := parseInt(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if c.Query("active_only") == "true" {
		activeOnly = true
	}

	// Get plans from database
	plans, total, err := s.listPlans(c.Request.Context(), page, limit, activeOnly)
	if err != nil {
		logrus.Errorf("Failed to list plans: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		telemetry.RecordPlanOperation("list", "db_error")
		return
	}

	response := PlanListResponse{
		Plans: plans,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	c.JSON(http.StatusOK, response)
	telemetry.RecordPlanOperation("list", "success")
}

func (s *Service) GetActivePlans(c *gin.Context) {
	// Try cache first
	cacheKey := "plans:active"
	cached, err := s.cache.Get(c.Request.Context(), cacheKey)
	if err == nil && cached != "" {
		var plans []Plan
		if err := json.Unmarshal([]byte(cached), &plans); err == nil {
			c.JSON(http.StatusOK, plans)
			telemetry.RecordPlanOperation("get_active", "cache_hit")
			return
		}
	}

	// Get from database
	plans, err := s.getActivePlans(c.Request.Context())
	if err != nil {
		logrus.Errorf("Failed to get active plans: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		telemetry.RecordPlanOperation("get_active", "db_error")
		return
	}

	// Cache active plans
	s.cacheActivePlans(c.Request.Context(), plans)

	c.JSON(http.StatusOK, plans)
	telemetry.RecordPlanOperation("get_active", "success")
}

// ComparePlans compares multiple plans and provides analysis
func (s *Service) ComparePlans(c *gin.Context) {
	planIDs := c.QueryArray("plan_ids")
	if len(planIDs) < 2 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "At least 2 plan IDs are required for comparison",
			Code:    "INSUFFICIENT_PLANS",
			Details: "Please provide at least 2 plan IDs to compare",
		})
		return
	}

	if len(planIDs) > 5 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Too many plans for comparison",
			Code:    "TOO_MANY_PLANS",
			Details: "Maximum 5 plans can be compared at once",
		})
		return
	}

	// Get plans from database
	plans := make([]Plan, 0, len(planIDs))
	for _, id := range planIDs {
		plan, err := s.getPlanByID(c.Request.Context(), id)
		if err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, ErrorResponse{
					Error:   fmt.Sprintf("Plan with ID %s not found", id),
					Code:    "PLAN_NOT_FOUND",
					Details: fmt.Sprintf("Plan ID: %s", id),
				})
				return
			}
			logrus.Errorf("Failed to get plan %s: %v", id, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Internal server error",
				Code:  "DB_ERROR",
			})
			return
		}
		plans = append(plans, *plan)
	}

	// Generate comparison
	comparison := s.generatePlanComparison(plans)

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Plan comparison generated successfully",
		Data:    comparison,
	})
	telemetry.RecordPlanOperation("compare", "success")
}

// GetPlanAnalytics provides comprehensive analytics for a specific plan
func (s *Service) GetPlanAnalytics(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Plan ID is required",
			Code:  "MISSING_PLAN_ID",
		})
		return
	}

	// Get plan details
	plan, err := s.getPlanByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Plan not found",
				Code:    "PLAN_NOT_FOUND",
				Details: fmt.Sprintf("Plan ID: %s", id),
			})
			return
		}
		logrus.Errorf("Failed to get plan: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Internal server error",
			Code:  "DB_ERROR",
		})
		return
	}

	// Generate analytics
	analytics, err := s.generatePlanAnalytics(c.Request.Context(), plan)
	if err != nil {
		logrus.Errorf("Failed to generate plan analytics: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to generate analytics",
			Code:  "ANALYTICS_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Plan analytics generated successfully",
		Data:    analytics,
	})
	telemetry.RecordPlanOperation("analytics", "success")
}

// Helper methods
func (s *Service) createPlan(ctx context.Context, plan *Plan) error {
	// Convert features map to JSON bytes for PostgreSQL JSONB
	var featuresBytes []byte
	var err error
	if plan.Features != nil {
		featuresBytes, err = json.Marshal(plan.Features)
		if err != nil {
			return fmt.Errorf("failed to marshal features to JSON: %w", err)
		}
	}

	query := `
		INSERT INTO plans (id, name, description, price, currency, billing_cycle, 
			features, max_usage_per_day, max_usage_per_month, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err = s.db.ExecContext(ctx, query, plan.ID, plan.Name, plan.Description, plan.Price,
		plan.Currency, plan.BillingCycle, featuresBytes, plan.MaxUsagePerDay,
		plan.MaxUsagePerMonth, plan.IsActive, plan.CreatedAt, plan.UpdatedAt)
	return err
}

func (s *Service) getPlanByID(ctx context.Context, id string) (*Plan, error) {
	query := `
		SELECT id, name, description, price, currency, billing_cycle, features,
			max_usage_per_day, max_usage_per_month, is_active, created_at, updated_at
		FROM plans WHERE id = $1
	`
	var plan Plan
	var featuresBytes []byte
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&plan.ID, &plan.Name, &plan.Description, &plan.Price, &plan.Currency,
		&plan.BillingCycle, &featuresBytes, &plan.MaxUsagePerDay, &plan.MaxUsagePerMonth,
		&plan.IsActive, &plan.CreatedAt, &plan.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Parse features JSON if not null
	if featuresBytes != nil {
		if err := json.Unmarshal(featuresBytes, &plan.Features); err != nil {
			return nil, fmt.Errorf("failed to parse features JSON: %w", err)
		}
	}

	return &plan, nil
}

func (s *Service) updatePlan(ctx context.Context, plan *Plan) error {
	// Convert features map to JSON bytes for PostgreSQL JSONB
	var featuresBytes []byte
	var err error
	if plan.Features != nil {
		featuresBytes, err = json.Marshal(plan.Features)
		if err != nil {
			return fmt.Errorf("failed to marshal features to JSON: %w", err)
		}
	}

	query := `
		UPDATE plans 
		SET name = $1, description = $2, price = $3, currency = $4, billing_cycle = $5,
			features = $6, max_usage_per_day = $7, max_usage_per_month = $8, 
			is_active = $9, updated_at = $10
		WHERE id = $11
	`
	_, err = s.db.ExecContext(ctx, query, plan.Name, plan.Description, plan.Price,
		plan.Currency, plan.BillingCycle, featuresBytes, plan.MaxUsagePerDay,
		plan.MaxUsagePerMonth, plan.IsActive, plan.UpdatedAt, plan.ID)
	return err
}

func (s *Service) deletePlan(ctx context.Context, id string) error {
	query := `DELETE FROM plans WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

func (s *Service) listPlans(ctx context.Context, page, limit int, activeOnly bool) ([]Plan, int, error) {
	offset := (page - 1) * limit

	// Build WHERE clause and args
	var whereClause string
	var args []interface{}

	if activeOnly {
		whereClause = "WHERE is_active = true"
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM plans %s", whereClause)
	var total int
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get plans with proper parameter indexing
	var query string
	if activeOnly {
		query = `
			SELECT id, name, description, price, currency, billing_cycle, features,
				max_usage_per_day, max_usage_per_month, is_active, created_at, updated_at
			FROM plans 
			WHERE is_active = true
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
	} else {
		query = `
			SELECT id, name, description, price, currency, billing_cycle, features,
				max_usage_per_day, max_usage_per_month, is_active, created_at, updated_at
			FROM plans 
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
	}

	args = []interface{}{limit, offset}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var plan Plan
		var featuresBytes []byte
		err := rows.Scan(
			&plan.ID, &plan.Name, &plan.Description, &plan.Price, &plan.Currency,
			&plan.BillingCycle, &featuresBytes, &plan.MaxUsagePerDay, &plan.MaxUsagePerMonth,
			&plan.IsActive, &plan.CreatedAt, &plan.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}

		// Parse features JSON if not null
		if featuresBytes != nil {
			if err := json.Unmarshal(featuresBytes, &plan.Features); err != nil {
				return nil, 0, fmt.Errorf("failed to parse features JSON: %w", err)
			}
		}

		plans = append(plans, plan)
	}

	return plans, total, nil
}

func (s *Service) getActivePlans(ctx context.Context) ([]Plan, error) {
	query := `
		SELECT id, name, description, price, currency, billing_cycle, features,
			max_usage_per_day, max_usage_per_month, is_active, created_at, updated_at
		FROM plans 
		WHERE is_active = true
		ORDER BY price ASC, created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var plan Plan
		var featuresBytes []byte
		err := rows.Scan(
			&plan.ID, &plan.Name, &plan.Description, &plan.Price, &plan.Currency,
			&plan.BillingCycle, &featuresBytes, &plan.MaxUsagePerDay, &plan.MaxUsagePerMonth,
			&plan.IsActive, &plan.CreatedAt, &plan.UpdatedAt)
		if err != nil {
			return nil, err
		}

		// Parse features JSON if not null
		if featuresBytes != nil {
			if err := json.Unmarshal(featuresBytes, &plan.Features); err != nil {
				return nil, fmt.Errorf("failed to parse features JSON: %w", err)
			}
		}

		plans = append(plans, plan)
	}

	return plans, nil
}

func (s *Service) planNameExists(ctx context.Context, name string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM plans WHERE name = $1)`
	var exists bool
	err := s.db.QueryRowContext(ctx, query, name).Scan(&exists)
	return exists, err
}

func (s *Service) planHasActiveSubscriptions(ctx context.Context, planID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM subscriptions WHERE plan_id = $1 AND status = 'active')`
	var exists bool
	err := s.db.QueryRowContext(ctx, query, planID).Scan(&exists)
	return exists, err
}

// Caching methods
func (s *Service) cachePlan(ctx context.Context, plan *Plan) {
	key := fmt.Sprintf("plan:%s", plan.ID)
	data, err := json.Marshal(plan)
	if err != nil {
		logrus.Errorf("Failed to marshal plan for cache: %v", err)
		return
	}

	// Cache for 1 hour
	if err := s.cache.Set(ctx, key, string(data), time.Hour); err != nil {
		logrus.Errorf("Failed to cache plan: %v", err)
	}

	// Invalidate active plans cache
	s.cache.Del(ctx, "plans:active")
}

func (s *Service) getCachedPlan(ctx context.Context, id string) (*Plan, error) {
	key := fmt.Sprintf("plan:%s", id)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var plan Plan
	if err := json.Unmarshal([]byte(data), &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *Service) removeCachedPlan(ctx context.Context, id string) {
	key := fmt.Sprintf("plan:%s", id)
	s.cache.Del(ctx, key)
	s.cache.Del(ctx, "plans:active")
}

func (s *Service) cacheActivePlans(ctx context.Context, plans []Plan) {
	data, err := json.Marshal(plans)
	if err != nil {
		logrus.Errorf("Failed to marshal active plans for cache: %v", err)
		return
	}

	// Cache for 30 minutes
	if err := s.cache.Set(ctx, "plans:active", string(data), 30*time.Minute); err != nil {
		logrus.Errorf("Failed to cache active plans: %v", err)
	}
}

// Utility functions
func generateID() string {
	// Generate a UUID v4 for the plan ID
	id := uuid.New()
	return id.String()
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

// validatePlanRequest validates the plan request and returns detailed error messages
func (s *Service) validatePlanRequest(req interface{}) error {
	if err := s.validator.Struct(req); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var errorMessages []string
			for _, fieldError := range validationErrors {
				switch fieldError.Tag() {
				case "required":
					errorMessages = append(errorMessages, fmt.Sprintf("%s is required", fieldError.Field()))
				case "min":
					errorMessages = append(errorMessages, fmt.Sprintf("%s must be at least %s", fieldError.Field(), fieldError.Param()))
				case "len":
					errorMessages = append(errorMessages, fmt.Sprintf("%s must be exactly %s characters", fieldError.Field(), fieldError.Param()))
				case "oneof":
					errorMessages = append(errorMessages, fmt.Sprintf("%s must be one of: %s", fieldError.Field(), fieldError.Param()))
				default:
					errorMessages = append(errorMessages, fmt.Sprintf("%s is invalid", fieldError.Field()))
				}
			}
			return fmt.Errorf("validation failed: %s", errorMessages[0])
		}
		return err
	}
	return nil
}

// generatePlanComparison creates a detailed comparison of plans
func (s *Service) generatePlanComparison(plans []Plan) PlanComparison {
	comparison := PlanComparison{
		Plans: make([]PlanComparisonItem, 0, len(plans)),
	}

	var minPrice, maxPrice float64
	var cheapestPlan, mostExpensivePlan string

	for i, plan := range plans {
		// Calculate monthly cost
		monthlyCost := s.calculateMonthlyCost(plan)
		yearlyCost := s.calculateYearlyCost(plan)

		// Track price extremes
		if i == 0 || monthlyCost < minPrice {
			minPrice = monthlyCost
			cheapestPlan = plan.Name
		}
		if i == 0 || monthlyCost > maxPrice {
			maxPrice = monthlyCost
			mostExpensivePlan = plan.Name
		}

		// Create feature matrix
		featureMatrix := s.createFeatureMatrix(plan)

		// Calculate cost per feature
		costPerFeature := s.calculateCostPerFeature(plan, monthlyCost)

		item := PlanComparisonItem{
			Plan:          plan,
			FeatureMatrix: featureMatrix,
			PriceAnalysis: PriceAnalysis{
				MonthlyCost:    monthlyCost,
				YearlyCost:     yearlyCost,
				CostPerFeature: costPerFeature,
			},
		}

		// Add recommendation
		if monthlyCost == minPrice {
			item.Recommendation = "Best value for money"
		} else if len(plan.Features) > 5 {
			item.Recommendation = "Feature-rich option"
		}

		comparison.Plans = append(comparison.Plans, item)
	}

	// Set summary
	comparison.Summary = PlanComparisonSummary{
		CheapestPlan:      cheapestPlan,
		MostExpensivePlan: mostExpensivePlan,
		PriceRange:        maxPrice - minPrice,
	}

	// Determine best value plan (lowest cost per feature)
	var bestValuePlan string
	var lowestCostPerFeature float64
	for _, item := range comparison.Plans {
		if item.PriceAnalysis.CostPerFeature > 0 &&
			(bestValuePlan == "" || item.PriceAnalysis.CostPerFeature < lowestCostPerFeature) {
			bestValuePlan = item.Plan.Name
			lowestCostPerFeature = item.PriceAnalysis.CostPerFeature
		}
	}
	comparison.Summary.BestValuePlan = bestValuePlan

	return comparison
}

// Helper methods for plan comparison
func (s *Service) calculateMonthlyCost(plan Plan) float64 {
	switch plan.BillingCycle {
	case "monthly":
		return plan.Price
	case "yearly":
		return plan.Price / 12
	case "weekly":
		return plan.Price * 4.33 // Average weeks per month
	case "daily":
		return plan.Price * 30.44 // Average days per month
	default:
		return plan.Price
	}
}

func (s *Service) calculateYearlyCost(plan Plan) float64 {
	switch plan.BillingCycle {
	case "yearly":
		return plan.Price
	case "monthly":
		return plan.Price * 12
	case "weekly":
		return plan.Price * 52
	case "daily":
		return plan.Price * 365
	default:
		return plan.Price * 12
	}
}

func (s *Service) createFeatureMatrix(plan Plan) map[string]interface{} {
	matrix := make(map[string]interface{})

	// Add basic plan info
	matrix["name"] = plan.Name
	matrix["price"] = plan.Price
	matrix["billing_cycle"] = plan.BillingCycle

	// Add features
	if plan.Features != nil {
		for key, value := range plan.Features {
			matrix[key] = value
		}
	}

	// Add usage limits
	if plan.MaxUsagePerDay != nil {
		matrix["max_usage_per_day"] = *plan.MaxUsagePerDay
	}
	if plan.MaxUsagePerMonth != nil {
		matrix["max_usage_per_month"] = *plan.MaxUsagePerMonth
	}

	return matrix
}

func (s *Service) calculateCostPerFeature(plan Plan, monthlyCost float64) float64 {
	if plan.Features == nil || len(plan.Features) == 0 {
		return monthlyCost
	}

	// Count boolean features that are true
	featureCount := 0
	for _, value := range plan.Features {
		if boolValue, ok := value.(bool); ok && boolValue {
			featureCount++
		}
	}

	if featureCount == 0 {
		return monthlyCost
	}

	return monthlyCost / float64(featureCount)
}

// generatePlanAnalytics creates comprehensive analytics for a plan
func (s *Service) generatePlanAnalytics(ctx context.Context, plan *Plan) (*PlanAnalytics, error) {
	analytics := &PlanAnalytics{
		PlanID:   plan.ID,
		PlanName: plan.Name,
	}

	// Get usage statistics
	usageStats, err := s.getUsageStatistics(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage statistics: %w", err)
	}
	analytics.UsageStats = *usageStats

	// Get popularity metrics
	popularity, err := s.getPopularityMetrics(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get popularity metrics: %w", err)
	}
	analytics.Popularity = *popularity

	// Get performance metrics
	performance, err := s.getPerformanceMetrics(ctx, plan.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get performance metrics: %w", err)
	}
	analytics.Performance = *performance

	// Generate recommendations
	analytics.Recommendations = s.generateRecommendations(analytics)

	return analytics, nil
}

// getUsageStatistics retrieves usage statistics for a plan
func (s *Service) getUsageStatistics(ctx context.Context, planID string) (*UsageStatistics, error) {
	// Get subscription counts
	var totalSubs, activeSubs int
	err := s.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*) as total_subscriptions,
			COUNT(CASE WHEN status = 'active' THEN 1 END) as active_subscriptions
		FROM subscriptions 
		WHERE plan_id = $1
	`, planID).Scan(&totalSubs, &activeSubs)
	if err != nil {
		return nil, err
	}

	// Get average usage (simplified - in real implementation, you'd query usage_logs)
	avgUsagePerDay := 0.0
	avgUsagePerMonth := 0.0
	if activeSubs > 0 {
		// This is a placeholder - real implementation would calculate from usage_logs
		avgUsagePerDay = 50.0
		avgUsagePerMonth = 1500.0
	}

	return &UsageStatistics{
		TotalSubscriptions:   totalSubs,
		ActiveSubscriptions:  activeSubs,
		AverageUsagePerDay:   avgUsagePerDay,
		AverageUsagePerMonth: avgUsagePerMonth,
	}, nil
}

// getPopularityMetrics retrieves popularity metrics for a plan
func (s *Service) getPopularityMetrics(ctx context.Context, planID string) (*PopularityMetrics, error) {
	// Calculate subscription growth (simplified)
	growth := 0.0
	retentionRate := 0.0
	churnRate := 0.0

	// Get subscription counts for different time periods
	var currentMonth, lastMonth int
	err := s.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(CASE WHEN created_at >= date_trunc('month', CURRENT_DATE) THEN 1 END) as current_month,
			COUNT(CASE WHEN created_at >= date_trunc('month', CURRENT_DATE - INTERVAL '1 month') 
				AND created_at < date_trunc('month', CURRENT_DATE) THEN 1 END) as last_month
		FROM subscriptions 
		WHERE plan_id = $1
	`, planID).Scan(&currentMonth, &lastMonth)
	if err != nil {
		return nil, err
	}

	// Calculate growth rate
	if lastMonth > 0 {
		growth = float64(currentMonth-lastMonth) / float64(lastMonth) * 100
	}

	// Determine trending
	trending := "stable"
	if growth > 10 {
		trending = "up"
	} else if growth < -10 {
		trending = "down"
	}

	return &PopularityMetrics{
		SubscriptionGrowth: growth,
		RetentionRate:      retentionRate,
		ChurnRate:          churnRate,
		Trending:           trending,
	}, nil
}

// getPerformanceMetrics retrieves performance metrics for a plan
func (s *Service) getPerformanceMetrics(ctx context.Context, planID string) (*PerformanceMetrics, error) {
	// Get revenue generated - fix column ambiguity by specifying table alias
	var revenue float64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(pt.amount), 0)
		FROM payment_transactions pt
		JOIN subscriptions s ON pt.subscription_id = s.id
		WHERE s.plan_id = $1 AND pt.status = 'completed'
	`, planID).Scan(&revenue)
	if err != nil {
		return nil, err
	}

	// Calculate average lifetime (simplified)
	avgLifetime := 0.0
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (end_date - start_date))/86400), 0)
		FROM subscriptions 
		WHERE plan_id = $1 AND status IN ('cancelled', 'expired')
	`, planID).Scan(&avgLifetime)
	if err != nil {
		return nil, err
	}

	return &PerformanceMetrics{
		RevenueGenerated: revenue,
		AverageLifetime:  avgLifetime,
		ConversionRate:   0.0, // Placeholder
	}, nil
}

// generateRecommendations creates actionable recommendations based on analytics
func (s *Service) generateRecommendations(analytics *PlanAnalytics) []string {
	var recommendations []string

	// Usage-based recommendations
	if analytics.UsageStats.ActiveSubscriptions < 10 {
		recommendations = append(recommendations, "Consider promotional pricing to increase adoption")
	}

	if analytics.Popularity.Trending == "down" {
		recommendations = append(recommendations, "Review pricing strategy and feature set")
	}

	if analytics.Performance.AverageLifetime < 30 {
		recommendations = append(recommendations, "Focus on improving customer retention")
	}

	if analytics.UsageStats.AverageUsagePerMonth < 1000 {
		recommendations = append(recommendations, "Consider reducing usage limits or pricing")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Plan is performing well, maintain current strategy")
	}

	return recommendations
}
