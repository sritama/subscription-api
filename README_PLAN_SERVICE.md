# Plan Service Documentation

The Plan Service is a comprehensive service for managing subscription plans in the PayFlow system. It provides full CRUD operations for plans with caching, validation, and telemetry integration.

## Features

- **Full CRUD Operations**: Create, read, update, and delete subscription plans
- **Flexible Plan Structure**: Support for various billing cycles, features, and usage limits
- **Caching**: Redis-based caching for improved performance
- **Validation**: Comprehensive input validation for plan data
- **Telemetry**: Prometheus metrics for monitoring plan operations
- **Business Logic**: Prevents deletion of plans with active subscriptions

## Plan Structure

```go
type Plan struct {
    ID              string                 `json:"id"`
    Name            string                 `json:"name"`
    Description     *string                `json:"description"`
    Price           float64                `json:"price"`
    Currency        string                 `json:"currency"`
    BillingCycle    string                 `json:"billing_cycle"`
    Features        map[string]interface{} `json:"features"`
    MaxUsagePerDay  *int                   `json:"max_usage_per_day"`
    MaxUsagePerMonth *int                  `json:"max_usage_per_month"`
    IsActive        bool                   `json:"is_active"`
    CreatedAt       time.Time             `json:"created_at"`
    UpdatedAt       time.Time             `json:"updated_at"`
}
```

## API Endpoints

### Create Plan
- **POST** `/api/v1/plans/`
- **Body**: `CreatePlanRequest`
- **Response**: Created `Plan` object

### List Plans
- **GET** `/api/v1/plans/`
- **Query Parameters**:
  - `page`: Page number (default: 1)
  - `limit`: Items per page (default: 20, max: 100)
  - `active_only`: Filter active plans only (default: false)
- **Response**: `PlanListResponse` with pagination

### Get Active Plans
- **GET** `/api/v1/plans/active`
- **Response**: Array of active plans (cached for 30 minutes)

### Get Plan by ID
- **GET** `/api/v1/plans/:id`
- **Response**: `Plan` object

### Update Plan
- **PUT** `/api/v1/plans/:id`
- **Body**: `UpdatePlanRequest` (partial updates supported)
- **Response**: Updated `Plan` object

### Delete Plan
- **DELETE** `/api/v1/plans/:id`
- **Validation**: Cannot delete plans with active subscriptions
- **Response**: Success message

## Request/Response Types

### CreatePlanRequest
```go
type CreatePlanRequest struct {
    Name            string                 `json:"name" binding:"required"`
    Description     *string                `json:"description"`
    Price           float64                `json:"price" binding:"required,min=0"`
    Currency        string                 `json:"currency" binding:"required,len=3"`
    BillingCycle    string                 `json:"billing_cycle" binding:"required,oneof=monthly yearly weekly daily"`
    Features        map[string]interface{} `json:"features"`
    MaxUsagePerDay  *int                   `json:"max_usage_per_day" binding:"min=0"`
    MaxUsagePerMonth *int                  `json:"max_usage_per_month" binding:"min=0"`
    IsActive        *bool                  `json:"is_active"`
}
```

### UpdatePlanRequest
```go
type UpdatePlanRequest struct {
    Name            *string                `json:"name"`
    Description     *string                `json:"description"`
    Price           *float64               `json:"price" binding:"omitempty,min=0"`
    Currency        *string                `json:"currency" binding:"omitempty,len=3"`
    BillingCycle    *string                `json:"billing_cycle" binding:"omitempty,oneof=monthly yearly weekly daily"`
    Features        *map[string]interface{} `json:"features"`
    MaxUsagePerDay  *int                   `json:"max_usage_per_day" binding:"omitempty,min=0"`
    MaxUsagePerMonth *int                  `json:"max_usage_per_month" binding:"omitempty,min=0"`
    IsActive        *bool                  `json:"is_active"`
}
```

## Validation Rules

- **Name**: Required, must be unique
- **Price**: Required, must be non-negative
- **Currency**: Required, must be exactly 3 characters (e.g., "USD", "EUR")
- **Billing Cycle**: Required, must be one of: "daily", "weekly", "monthly", "yearly"
- **Usage Limits**: Optional, must be non-negative if provided
- **Features**: Flexible JSON object for plan-specific features

## Caching Strategy

- **Individual Plans**: Cached for 1 hour
- **Active Plans List**: Cached for 30 minutes
- **Cache Invalidation**: Automatic invalidation on plan updates/deletes

## Business Rules

1. **Plan Names**: Must be unique across all plans
2. **Deletion Protection**: Plans with active subscriptions cannot be deleted
3. **Active Status**: New plans are active by default
4. **Usage Limits**: -1 indicates unlimited usage

## Example Usage

### Creating a Basic Plan
```bash
curl -X POST http://localhost:8080/api/v1/plans/ \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Basic Plan",
    "description": "Basic subscription with limited features",
    "price": 9.99,
    "currency": "USD",
    "billing_cycle": "monthly",
    "features": {
      "feature1": true,
      "feature2": false,
      "max_users": 5
    },
    "max_usage_per_day": 100,
    "max_usage_per_month": 3000
  }'
```

### Updating a Plan
```bash
curl -X PUT http://localhost:8080/api/v1/plans/plan_123 \
  -H "Content-Type: application/json" \
  -d '{
    "price": 14.99,
    "description": "Updated description"
  }'
```

### Getting Active Plans
```bash
curl http://localhost:8080/api/v1/plans/active
```

## Metrics

The service provides Prometheus metrics for monitoring:

- `plan_operations_total`: Counter for plan operations with labels:
  - `operation`: create, get, update, delete, list, get_active
  - `status`: success, validation_error, db_error, conflict, not_found

## Error Handling

- **400 Bad Request**: Validation errors
- **404 Not Found**: Plan not found
- **409 Conflict**: Plan name already exists or cannot delete
- **500 Internal Server Error**: Database or system errors

## Integration

The Plan Service integrates with:
- **Subscription Service**: Plans are referenced by subscriptions
- **Paywall Service**: Plans determine access levels and usage limits
- **Cache Service**: Redis for performance optimization
- **Telemetry Service**: Metrics and monitoring 