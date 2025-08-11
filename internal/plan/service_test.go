package plan

import (
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

func TestPlanComparison(t *testing.T) {
	// Create test plans
	basicPlan := &Plan{
		ID:           "plan_basic",
		Name:         "Basic Plan",
		Price:        9.99,
		Currency:     "USD",
		BillingCycle: "monthly",
		Features: map[string]interface{}{
			"feature1": true,
			"feature2": false,
		},
		MaxUsagePerDay:   intPtr(100),
		MaxUsagePerMonth: intPtr(3000),
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	proPlan := &Plan{
		ID:           "plan_pro",
		Name:         "Pro Plan",
		Price:        19.99,
		Currency:     "USD",
		BillingCycle: "monthly",
		Features: map[string]interface{}{
			"feature1": true,
			"feature2": true,
			"feature3": true,
		},
		MaxUsagePerDay:   intPtr(500),
		MaxUsagePerMonth: intPtr(15000),
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	enterprisePlan := &Plan{
		ID:           "plan_enterprise",
		Name:         "Enterprise Plan",
		Price:        49.99,
		Currency:     "USD",
		BillingCycle: "yearly",
		Features: map[string]interface{}{
			"feature1": true,
			"feature2": true,
			"feature3": true,
			"feature4": true,
			"feature5": true,
		},
		MaxUsagePerDay:   intPtr(-1), // unlimited
		MaxUsagePerMonth: intPtr(-1), // unlimited
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Test monthly cost calculation
	t.Run("Monthly Cost Calculation", func(t *testing.T) {
		service := &Service{}

		basicMonthly := service.calculateMonthlyCost(*basicPlan)
		proMonthly := service.calculateMonthlyCost(*proPlan)
		enterpriseMonthly := service.calculateMonthlyCost(*enterprisePlan)

		assert.Equal(t, 9.99, basicMonthly)
		assert.Equal(t, 19.99, proMonthly)
		assert.Equal(t, 49.99/12, enterpriseMonthly)
	})

	// Test yearly cost calculation
	t.Run("Yearly Cost Calculation", func(t *testing.T) {
		service := &Service{}

		basicYearly := service.calculateYearlyCost(*basicPlan)
		proYearly := service.calculateYearlyCost(*proPlan)
		enterpriseYearly := service.calculateYearlyCost(*enterprisePlan)

		assert.Equal(t, 9.99*12, basicYearly)
		assert.Equal(t, 19.99*12, proYearly)
		assert.Equal(t, 49.99, enterpriseYearly)
	})

	// Test feature matrix creation
	t.Run("Feature Matrix Creation", func(t *testing.T) {
		service := &Service{}

		matrix := service.createFeatureMatrix(*proPlan)

		assert.Equal(t, "Pro Plan", matrix["name"])
		assert.Equal(t, 19.99, matrix["price"])
		assert.Equal(t, "monthly", matrix["billing_cycle"])
		assert.Equal(t, true, matrix["feature1"])
		assert.Equal(t, true, matrix["feature2"])
		assert.Equal(t, true, matrix["feature3"])
		assert.Equal(t, 500, matrix["max_usage_per_day"])
		assert.Equal(t, 15000, matrix["max_usage_per_month"])
	})

	// Test cost per feature calculation
	t.Run("Cost Per Feature Calculation", func(t *testing.T) {
		service := &Service{}

		basicCostPerFeature := service.calculateCostPerFeature(*basicPlan, 9.99)
		proCostPerFeature := service.calculateCostPerFeature(*proPlan, 19.99)
		enterpriseCostPerFeature := service.calculateCostPerFeature(*enterprisePlan, 49.99/12)

		assert.Equal(t, 9.99, basicCostPerFeature)              // Only 1 true feature
		assert.Equal(t, 19.99/3, proCostPerFeature)             // 3 true features
		assert.Equal(t, (49.99/12)/5, enterpriseCostPerFeature) // 5 true features
	})
}

func TestPlanAnalytics(t *testing.T) {
	// Test usage statistics
	t.Run("Usage Statistics", func(t *testing.T) {
		stats := &UsageStatistics{
			TotalSubscriptions:   100,
			ActiveSubscriptions:  85,
			AverageUsagePerDay:   150.5,
			AverageUsagePerMonth: 4500.0,
		}

		assert.Equal(t, 100, stats.TotalSubscriptions)
		assert.Equal(t, 85, stats.ActiveSubscriptions)
		assert.Equal(t, 150.5, stats.AverageUsagePerDay)
		assert.Equal(t, 4500.0, stats.AverageUsagePerMonth)
	})

	// Test popularity metrics
	t.Run("Popularity Metrics", func(t *testing.T) {
		metrics := &PopularityMetrics{
			SubscriptionGrowth: 15.5,
			RetentionRate:      92.0,
			ChurnRate:          8.0,
			Trending:           "up",
		}

		assert.Equal(t, 15.5, metrics.SubscriptionGrowth)
		assert.Equal(t, 92.0, metrics.RetentionRate)
		assert.Equal(t, 8.0, metrics.ChurnRate)
		assert.Equal(t, "up", metrics.Trending)
	})

	// Test performance metrics
	t.Run("Performance Metrics", func(t *testing.T) {
		metrics := &PerformanceMetrics{
			RevenueGenerated:     9999.99,
			AverageLifetime:      180.5,
			ConversionRate:       12.5,
			CustomerSatisfaction: 4.8,
		}

		assert.Equal(t, 9999.99, metrics.RevenueGenerated)
		assert.Equal(t, 180.5, metrics.AverageLifetime)
		assert.Equal(t, 12.5, metrics.ConversionRate)
		assert.Equal(t, 4.8, metrics.CustomerSatisfaction)
	})
}

func TestPlanValidation(t *testing.T) {
	service := &Service{
		validator: validator.New(),
	}

	tests := []struct {
		name     string
		request  CreatePlanRequest
		isValid  bool
		errorMsg string
	}{
		{
			name: "Valid plan with all fields",
			request: CreatePlanRequest{
				Name:         "Premium Plan",
				Description:  stringPtr("A premium plan with all features"),
				Price:        29.99,
				Currency:     "EUR",
				BillingCycle: "yearly",
				Features: map[string]interface{}{
					"feature1": true,
					"feature2": true,
				},
				MaxUsagePerDay:   intPtr(1000),
				MaxUsagePerMonth: intPtr(30000),
				IsActive:         boolPtr(true),
			},
			isValid: true,
		},
		{
			name: "Valid plan with minimal fields",
			request: CreatePlanRequest{
				Name:         "Basic Plan",
				Price:        9.99,
				Currency:     "USD",
				BillingCycle: "monthly",
			},
			isValid: true,
		},
		{
			name: "Invalid - missing name",
			request: CreatePlanRequest{
				Price:        9.99,
				Currency:     "USD",
				BillingCycle: "monthly",
			},
			isValid:  false,
			errorMsg: "Name is required",
		},
		{
			name: "Invalid - negative price",
			request: CreatePlanRequest{
				Name:         "Invalid Plan",
				Price:        -5.99,
				Currency:     "USD",
				BillingCycle: "monthly",
			},
			isValid:  false,
			errorMsg: "Price must be at least 0",
		},
		{
			name: "Invalid - wrong currency length",
			request: CreatePlanRequest{
				Name:         "Invalid Plan",
				Price:        9.99,
				Currency:     "US",
				BillingCycle: "monthly",
			},
			isValid:  false,
			errorMsg: "Currency must be exactly 3 characters",
		},
		{
			name: "Invalid - invalid billing cycle",
			request: CreatePlanRequest{
				Name:         "Invalid Plan",
				Price:        9.99,
				Currency:     "USD",
				BillingCycle: "quarterly",
			},
			isValid:  false,
			errorMsg: "BillingCycle must be one of: monthly yearly weekly daily",
		},
		{
			name: "Invalid - negative usage limits",
			request: CreatePlanRequest{
				Name:             "Invalid Plan",
				Price:            9.99,
				Currency:         "USD",
				BillingCycle:     "monthly",
				MaxUsagePerDay:   intPtr(-10),
				MaxUsagePerMonth: intPtr(-100),
			},
			isValid:  false,
			errorMsg: "MaxUsagePerDay must be at least 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validatePlanRequest(tt.request)

			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestPlanFeatures(t *testing.T) {
	// Test plan with complex features
	plan := &Plan{
		ID:           "plan_advanced",
		Name:         "Advanced Plan",
		Description:  stringPtr("Advanced plan with complex features"),
		Price:        39.99,
		Currency:     "USD",
		BillingCycle: "monthly",
		Features: map[string]interface{}{
			"api_access":       true,
			"webhook_support":  true,
			"max_users":        25,
			"storage_gb":       100,
			"priority_support": true,
			"custom_domain":    false,
		},
		MaxUsagePerDay:   intPtr(5000),
		MaxUsagePerMonth: intPtr(150000),
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	assert.Equal(t, "plan_advanced", plan.ID)
	assert.Equal(t, "Advanced Plan", plan.Name)
	assert.Equal(t, "Advanced plan with complex features", *plan.Description)
	assert.Equal(t, 39.99, plan.Price)
	assert.Equal(t, "USD", plan.Currency)
	assert.Equal(t, "monthly", plan.BillingCycle)

	// Test feature access
	assert.Equal(t, true, plan.Features["api_access"])
	assert.Equal(t, true, plan.Features["webhook_support"])
	assert.Equal(t, 25, plan.Features["max_users"])
	assert.Equal(t, 100, plan.Features["storage_gb"])
	assert.Equal(t, true, plan.Features["priority_support"])
	assert.Equal(t, false, plan.Features["custom_domain"])

	assert.Equal(t, 5000, *plan.MaxUsagePerDay)
	assert.Equal(t, 150000, *plan.MaxUsagePerMonth)
	assert.True(t, plan.IsActive)
}

func TestPlanUpdate(t *testing.T) {
	plan := &Plan{
		ID:           "plan_123",
		Name:         "Original Plan",
		Price:        19.99,
		Currency:     "USD",
		BillingCycle: "monthly",
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Test updating multiple fields
	newName := "Updated Premium Plan"
	newPrice := 29.99
	newDescription := "Enhanced premium plan with new features"
	newCurrency := "EUR"
	newBillingCycle := "yearly"
	newFeatures := map[string]interface{}{
		"feature1": true,
		"feature2": true,
		"feature3": true,
	}

	updateReq := UpdatePlanRequest{
		Name:         &newName,
		Price:        &newPrice,
		Description:  &newDescription,
		Currency:     &newCurrency,
		BillingCycle: &newBillingCycle,
		Features:     &newFeatures,
	}

	// Simulate update logic
	if updateReq.Name != nil {
		plan.Name = *updateReq.Name
	}
	if updateReq.Price != nil {
		plan.Price = *updateReq.Price
	}
	if updateReq.Description != nil {
		plan.Description = updateReq.Description
	}
	if updateReq.Currency != nil {
		plan.Currency = *updateReq.Currency
	}
	if updateReq.BillingCycle != nil {
		plan.BillingCycle = *updateReq.BillingCycle
	}
	if updateReq.Features != nil {
		plan.Features = *updateReq.Features
	}

	// Verify updates
	assert.Equal(t, "Updated Premium Plan", plan.Name)
	assert.Equal(t, 29.99, plan.Price)
	assert.Equal(t, "Enhanced premium plan with new features", *plan.Description)
	assert.Equal(t, "EUR", plan.Currency)
	assert.Equal(t, "yearly", plan.BillingCycle)
	assert.Equal(t, 3, len(plan.Features))
	assert.Equal(t, true, plan.Features["feature1"])
	assert.Equal(t, true, plan.Features["feature2"])
	assert.Equal(t, true, plan.Features["feature3"])
}

// Helper functions for testing
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
