# PayFlow - Low Level Design Document

## Table of Contents
1. [Introduction](#introduction)
2. [System Overview](#system-overview)
3. [Architecture Design](#architecture-design)
4. [Database Design](#database-design)
5. [API Design](#api-design)
6. [Core Components](#core-components)
7. [Data Flow](#data-flow)
8. [Security Considerations](#security-considerations)
9. [Performance Considerations](#performance-considerations)
10. [Scalability Considerations](#scalability-considerations)
11. [Error Handling](#error-handling)
12. [Monitoring and Observability](#monitoring-and-observability)
13. [Deployment and Infrastructure](#deployment-and-infrastructure)
14. [Testing Strategy](#testing-strategy)
15. [Future Enhancements](#future-enhancements)

## Introduction

PayFlow is a scalable paywall system designed to handle subscription-based content monetization for digital platforms. The system provides comprehensive plan management, subscription handling, payment processing, and usage tracking capabilities. This document outlines the low-level design and implementation details of the PayFlow system.

### Problem Statement
Digital content platforms need a robust, scalable system to:
- Manage multiple subscription plans with varying features and pricing
- Handle user subscriptions and billing cycles
- Track usage patterns and enforce limits
- Process payments securely
- Provide analytics and insights for business decisions

### Solution Overview
PayFlow addresses these challenges through a microservices-based architecture with:
- RESTful API endpoints for all operations
- PostgreSQL database with JSONB support for flexible feature storage
- Redis caching for performance optimization
- Comprehensive validation and error handling
- Telemetry and monitoring capabilities

## System Overview

### Core Functionality
1. **Plan Management**: Create, read, update, and delete subscription plans
2. **Subscription Handling**: Manage user subscriptions with lifecycle management
3. **Payment Processing**: Handle payment transactions and gateway integration
4. **Usage Tracking**: Monitor and log user actions and content consumption
5. **Analytics**: Generate insights on plan performance and user behavior
6. **Webhook Management**: Handle external system notifications

### Key Features
- **Flexible Plan Structure**: JSONB-based feature system allowing dynamic plan configurations
- **Multi-currency Support**: Built-in support for different currencies
- **Billing Cycle Flexibility**: Support for daily, weekly, monthly, and yearly billing
- **Usage Limits**: Configurable daily and monthly usage restrictions
- **Real-time Analytics**: Live metrics and performance indicators
- **Caching Layer**: Redis-based caching for improved response times

## Architecture Design

### High-Level Architecture
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Client Apps   │    │   Admin Panel   │    │   Third-party   │
│                 │    │                 │    │   Integrations  │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                    ┌─────────────▼─────────────┐
                    │      API Gateway         │
                    │      (Gin Router)        │
                    └─────────────┬─────────────┘
                                 │
                    ┌─────────────▼─────────────┐
                    │     Business Logic       │
                    │     (Service Layer)      │
                    └─────────────┬─────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
┌─────────▼─────────┐  ┌─────────▼─────────┐  ┌─────────▼─────────┐
│   PostgreSQL DB   │  │   Redis Cache    │  │   Telemetry      │
│                   │  │                   │  │   (Prometheus)   │
└───────────────────┘  └───────────────────┘  └───────────────────┘
```

### Service Layer Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Layer (Gin)                        │
├─────────────────────────────────────────────────────────────┤
│                    Middleware Layer                        │
│  • Authentication • Validation • Rate Limiting • Logging   │
├─────────────────────────────────────────────────────────────┤
│                    Service Layer                           │
│  • Plan Service • Subscription Service • Payment Service   │
│  • Usage Service • Analytics Service • Webhook Service     │
├─────────────────────────────────────────────────────────────┤
│                    Data Access Layer                       │
│  • Database Connection • Cache Client • Repository Pattern │
├─────────────────────────────────────────────────────────────┤
│                    Infrastructure Layer                     │
│  • PostgreSQL • Redis • Prometheus • OpenTelemetry        │
└─────────────────────────────────────────────────────────────┘
```

## Database Design

### Database Schema

#### Plans Table
```sql
CREATE TABLE plans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    billing_cycle VARCHAR(20) NOT NULL CHECK (billing_cycle IN ('monthly', 'yearly', 'weekly', 'daily')),
    features JSONB,
    max_usage_per_day INTEGER DEFAULT 100,
    max_usage_per_month INTEGER DEFAULT 3000,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Key Design Decisions:**
- **UUID Primary Keys**: Ensures global uniqueness and supports distributed systems
- **JSONB Features**: Provides flexibility for dynamic plan configurations without schema changes
- **Billing Cycle Constraints**: Enforces valid billing cycle values at database level
- **Usage Limits**: Configurable daily and monthly restrictions with sensible defaults
- **Audit Fields**: Created and updated timestamps for tracking changes

#### Subscriptions Table
```sql
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled', 'expired', 'suspended', 'pending')),
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE NOT NULL,
    auto_renew BOOLEAN DEFAULT true,
    payment_method_id VARCHAR(255),
    amount DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    trial_end TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Key Design Decisions:**
- **Referential Integrity**: Foreign key constraints ensure data consistency
- **Status Management**: Comprehensive subscription lifecycle states
- **Trial Support**: Built-in trial period handling
- **Auto-renewal**: Configurable automatic renewal behavior

#### Payment Transactions Table
```sql
CREATE TABLE payment_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'completed', 'failed', 'refunded')),
    payment_method VARCHAR(50),
    gateway_transaction_id VARCHAR(255),
    gateway_response JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Key Design Decisions:**
- **Flexible Payment Methods**: Support for various payment gateways
- **Gateway Integration**: JSONB storage for gateway-specific responses
- **Audit Trail**: Complete transaction history for compliance and debugging

#### Usage Logs Table
```sql
CREATE TABLE usage_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    content_id VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Key Design Decisions:**
- **Action Tracking**: Granular usage monitoring
- **Metadata Flexibility**: JSONB storage for action-specific data
- **Performance Optimization**: Indexed on user_id and created_at

### Database Indexes
```sql
-- Performance indexes
CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_end_date ON subscriptions(end_date);
CREATE INDEX idx_payment_transactions_user_id ON payment_transactions(user_id);
CREATE INDEX idx_payment_transactions_status ON payment_transactions(status);
CREATE INDEX idx_usage_logs_user_id ON usage_logs(user_id);
CREATE INDEX idx_usage_logs_created_at ON usage_logs(created_at);
CREATE INDEX idx_webhook_events_processed ON webhook_events(processed);
```

### Triggers
```sql
-- Auto-update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply to all tables
CREATE TRIGGER update_plans_updated_at BEFORE UPDATE ON plans FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_subscriptions_updated_at BEFORE UPDATE ON subscriptions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_payment_transactions_updated_at BEFORE UPDATE ON payment_transactions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

## API Design

### RESTful API Structure

#### Base URL
```
http://localhost:8080/api/v1
```

#### Authentication
- **Bearer Token**: JWT-based authentication for protected endpoints
- **API Key**: Alternative authentication method for service-to-service communication

#### Response Format
```json
{
  "message": "Operation completed successfully",
  "data": {},
  "error": null,
  "code": null,
  "details": null
}
```

#### Error Response Format
```json
{
  "message": null,
  "data": null,
  "error": "Error description",
  "code": "ERROR_CODE",
  "details": "Additional error details"
}
```

### Core Endpoints

#### Plan Management
```
GET    /plans/                    - List all plans
POST   /plans/                    - Create new plan
GET    /plans/{id}                - Get plan by ID
PUT    /plans/{id}                - Update plan
DELETE /plans/{id}                - Delete plan
GET    /plans/active              - Get active plans only
GET    /plans/compare             - Compare multiple plans
GET    /plans/{id}/analytics     - Get plan analytics
```

#### Subscription Management
```
GET    /subscriptions/            - List subscriptions
POST   /subscriptions/            - Create subscription
GET    /subscriptions/{id}        - Get subscription by ID
PUT    /subscriptions/{id}        - Update subscription
DELETE /subscriptions/{id}        - Cancel subscription
GET    /subscriptions/user/{id}   - Get user subscriptions
```

#### Payment Management
```
GET    /payments/                 - List payment transactions
POST   /payments/                 - Process payment
GET    /payments/{id}             - Get payment by ID
POST   /payments/{id}/refund      - Process refund
```

#### Usage Tracking
```
GET    /usage/                    - Get usage statistics
POST   /usage/log                 - Log usage action
GET    /usage/user/{id}           - Get user usage
```

### Request/Response Examples

#### Create Plan Request
```json
{
  "name": "Pro Plan",
  "description": "Professional plan with advanced features",
  "price": 19.99,
  "currency": "USD",
  "billing_cycle": "monthly",
  "features": {
    "feature1": true,
    "feature2": true,
    "feature3": true,
    "max_storage": "100GB",
    "api_calls": 10000
  },
  "max_usage_per_day": 200,
  "max_usage_per_month": 6000,
  "is_active": true
}
```

#### Plan Response
```json
{
  "id": "36028c50-bb70-47bf-ba6d-d7ca904aa41b",
  "name": "Pro Plan",
  "description": "Professional plan with advanced features",
  "price": 19.99,
  "currency": "USD",
  "billing_cycle": "monthly",
  "features": {
    "feature1": true,
    "feature2": true,
    "feature3": true,
    "max_storage": "100GB",
    "api_calls": 10000
  },
  "max_usage_per_day": 200,
  "max_usage_per_month": 6000,
  "is_active": true,
  "created_at": "2025-08-10T19:39:59.772811-07:00",
  "updated_at": "2025-08-10T19:39:59.772811-07:00"
}
```

## Core Components

### Service Layer Implementation

#### Plan Service
```go
type Service struct {
    db        *db.Connection
    cache     *cache.RedisClient
    validator *validator.Validate
}

// Core methods
func (s *Service) CreatePlan(c *gin.Context)
func (s *Service) GetPlan(c *gin.Context)
func (s *Service) UpdatePlan(c *gin.Context)
func (s *Service) DeletePlan(c *gin.Context)
func (s *Service) ListPlans(c *gin.Context)
func (s *Service) GetActivePlans(c *gin.Context)
func (s *Service) ComparePlans(c *gin.Context)
func (s *Service) GetPlanAnalytics(c *gin.Context)
```

**Key Features:**
- **Validation**: Comprehensive request validation using go-playground/validator
- **Caching**: Redis-based caching for frequently accessed plans
- **Analytics**: Built-in analytics generation for plan performance
- **Comparison**: Multi-plan comparison with feature matrices

#### Data Models
```go
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
```

### Caching Strategy

#### Redis Implementation
```go
type RedisClient struct {
    client *redis.Client
    ctx    context.Context
}

// Cache operations
func (r *RedisClient) Set(key string, value interface{}, expiration time.Duration)
func (r *RedisClient) Get(key string) (string, error)
func (r *RedisClient) Delete(key string) error
func (r *RedisClient) Exists(key string) (bool, error)
```

**Caching Patterns:**
- **Plan Caching**: Cache individual plans and active plan lists
- **TTL Management**: Configurable expiration times for different data types
- **Cache Invalidation**: Automatic cache updates on data modifications
- **Memory Optimization**: Efficient serialization and storage

### Validation Layer

#### Request Validation
```go
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
```

**Validation Rules:**
- **Required Fields**: Essential fields must be provided
- **Value Constraints**: Price must be positive, currency must be 3 characters
- **Enum Validation**: Billing cycle must be one of predefined values
- **Conditional Validation**: Usage limits are optional but must be positive when provided

## Data Flow

### Plan Creation Flow
```
1. Client Request → API Gateway
2. Request Validation → Service Layer
3. Business Logic → Plan Creation
4. Database Insert → PostgreSQL
5. Cache Update → Redis
6. Response → Client
```

### Plan Retrieval Flow
```
1. Client Request → API Gateway
2. Cache Check → Redis
3. If Cache Hit → Return Cached Data
4. If Cache Miss → Database Query → PostgreSQL
5. Cache Update → Redis
6. Response → Client
```

### Usage Tracking Flow
```
1. User Action → Usage Service
2. Usage Log → Database
3. Limit Check → Business Rules
4. Analytics Update → Real-time Metrics
5. Webhook Trigger → External Systems
```

## Security Considerations

### Authentication & Authorization
- **JWT Tokens**: Secure token-based authentication
- **Role-Based Access Control**: Different permission levels for different user types
- **API Key Management**: Secure API key generation and rotation

### Data Protection
- **Input Validation**: Comprehensive request validation to prevent injection attacks
- **SQL Injection Prevention**: Parameterized queries and proper escaping
- **XSS Protection**: Output encoding and sanitization

### API Security
- **Rate Limiting**: Protection against abuse and DoS attacks
- **CORS Configuration**: Proper cross-origin resource sharing settings
- **HTTPS Enforcement**: Secure communication over TLS

## Performance Considerations

### Database Optimization
- **Connection Pooling**: Efficient database connection management
- **Query Optimization**: Proper indexing and query planning
- **JSONB Performance**: Efficient JSON operations for features field

### Caching Strategy
- **Multi-Level Caching**: Application and database level caching
- **Cache Warming**: Proactive cache population for frequently accessed data
- **Cache Invalidation**: Smart cache update strategies

### Response Optimization
- **Pagination**: Efficient handling of large datasets
- **Field Selection**: Optional field inclusion/exclusion
- **Compression**: Response compression for large payloads

## Scalability Considerations

### Horizontal Scaling
- **Stateless Services**: Services can be scaled horizontally
- **Load Balancing**: Multiple service instances behind load balancer
- **Database Sharding**: Potential for database partitioning by user or plan

### Vertical Scaling
- **Resource Optimization**: Efficient memory and CPU usage
- **Connection Pooling**: Optimized database connection management
- **Async Processing**: Background processing for non-critical operations

### Microservices Architecture
- **Service Decomposition**: Logical separation of concerns
- **Independent Deployment**: Services can be deployed independently
- **Technology Flexibility**: Different services can use different technologies

## Error Handling

### Error Classification
```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Code    string `json:"code,omitempty"`
    Details string `json:"details,omitempty"`
}
```

**Error Categories:**
- **Validation Errors**: Input validation failures
- **Business Logic Errors**: Rule violations and constraints
- **Database Errors**: Connection and query failures
- **External Service Errors**: Third-party service failures

### Error Recovery
- **Retry Mechanisms**: Automatic retry for transient failures
- **Circuit Breaker**: Protection against cascading failures
- **Graceful Degradation**: Service continues with reduced functionality

### Logging and Monitoring
- **Structured Logging**: Consistent log format for analysis
- **Error Tracking**: Centralized error monitoring and alerting
- **Performance Metrics**: Response time and throughput monitoring

## Monitoring and Observability

### Metrics Collection
```go
// Prometheus metrics
var (
    planOperations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "payflow_plan_operations_total",
            Help: "Total number of plan operations",
        },
        []string{"operation", "status"},
    )
)
```

### Health Checks
```go
// Health endpoint
func HealthCheck(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "status": "healthy",
        "timestamp": time.Now(),
        "version": "1.0.0",
    })
}
```

### Distributed Tracing
- **OpenTelemetry Integration**: End-to-end request tracing
- **Performance Analysis**: Identify bottlenecks and optimization opportunities
- **Dependency Mapping**: Understand service dependencies and interactions

## Deployment and Infrastructure

### Containerization
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

### Environment Configuration
```yaml
# config.yaml
database:
  host: localhost
  port: 5432
  user: baabi
  password: ""
  dbname: paywall
  sslmode: disable

redis:
  host: localhost
  port: 6379
  password: ""
  db: 0

server:
  port: 8080
  host: "0.0.0.0"
```

### Infrastructure Requirements
- **PostgreSQL**: Version 12+ with JSONB support
- **Redis**: Version 6+ for caching
- **Go Runtime**: Version 1.21+
- **Network**: HTTP/HTTPS access on configured port

## Testing Strategy

### Unit Testing
```go
func TestCreatePlan(t *testing.T) {
    // Test plan creation with valid data
    // Test validation errors
    // Test database errors
    // Test cache operations
}
```

### Integration Testing
- **Database Integration**: Test database operations and constraints
- **Cache Integration**: Test Redis operations and cache invalidation
- **API Integration**: Test complete request-response cycles

### Performance Testing
- **Load Testing**: Simulate high concurrent usage
- **Stress Testing**: Test system limits and failure modes
- **Benchmark Testing**: Measure response times and throughput

## Future Enhancements

### Planned Features
1. **Multi-tenancy**: Support for multiple organizations
2. **Advanced Analytics**: Machine learning-based insights
3. **Payment Gateway Integration**: Direct payment processing
4. **Webhook Management**: Enhanced external system integration
5. **Mobile SDK**: Native mobile application support

### Technical Improvements
1. **GraphQL API**: Alternative to REST for complex queries
2. **Event Sourcing**: Complete audit trail and event replay
3. **CQRS Pattern**: Separate read and write models
4. **Microservices**: Further service decomposition
5. **Kubernetes Deployment**: Container orchestration and scaling

### Business Features
1. **Dynamic Pricing**: Real-time price adjustments
2. **A/B Testing**: Plan performance testing
3. **Customer Segmentation**: Targeted plan offerings
4. **Loyalty Programs**: Customer retention features
5. **Internationalization**: Multi-language and multi-currency support

## Conclusion

The PayFlow system provides a robust, scalable foundation for subscription-based content monetization. The architecture emphasizes:

- **Flexibility**: JSONB-based feature system allows dynamic plan configurations
- **Performance**: Multi-level caching and optimized database queries
- **Scalability**: Microservices architecture supporting horizontal scaling
- **Reliability**: Comprehensive error handling and monitoring
- **Security**: Multi-layered security with proper validation and authentication

The system is designed to handle growth from small-scale deployments to enterprise-level usage, with clear upgrade paths and enhancement opportunities. The modular architecture allows for incremental improvements and feature additions without disrupting existing functionality.

---

*Document Version: 1.0*  
*Last Updated: August 10, 2025*  
*Author: PayFlow Development Team* 