# Subscription API

A scalable paywall system designed to handle subscription-based content monetization for digital platforms. PayFlow provides comprehensive plan management, subscription handling, payment processing, and usage tracking capabilities.

## 🚀 Features

- **Flexible Plan Management**: Create, update, and manage subscription plans with dynamic features
- **Subscription Handling**: Complete subscription lifecycle management
- **Payment Processing**: Secure payment transaction handling
- **Usage Tracking**: Monitor and enforce usage limits
- **Real-time Analytics**: Comprehensive insights and performance metrics
- **Multi-currency Support**: Built-in support for different currencies
- **Flexible Billing Cycles**: Daily, weekly, monthly, and yearly billing options
- **Webhook Management**: External system integration capabilities

## 🏗️ Architecture

PayFlow is built with a microservices-based architecture:

- **API Layer**: RESTful HTTP API using Gin framework
- **Service Layer**: Business logic and validation
- **Data Layer**: PostgreSQL with JSONB support for flexible data storage
- **Cache Layer**: Redis for performance optimization
- **Monitoring**: Prometheus metrics and OpenTelemetry integration

## 🛠️ Technology Stack

- **Language**: Go 1.21+
- **Web Framework**: Gin
- **Database**: PostgreSQL 12+ with JSONB support
- **Cache**: Redis 6+
- **Validation**: go-playground/validator
- **UUID Generation**: google/uuid
- **Configuration**: YAML-based configuration
- **Testing**: Go testing framework

## 📋 Prerequisites

- Go 1.21 or higher
- PostgreSQL 12 or higher
- Redis 6 or higher
- Make (optional, for build automation)

## 🚀 Quick Start

### 1. Clone the Repository

```bash
git clone <your-repo-url>
cd payflow-api
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Configure Database

Create a PostgreSQL database and update the configuration in `configs/config.yaml`:

```yaml
database:
  host: localhost
  port: 5432
  user: your_username
  password: your_password
  dbname: paywall
  sslmode: disable
```

### 4. Run Database Migrations

```bash
# Connect to your PostgreSQL database and run the migration
psql -d paywall -f internal/db/migrations/001_initial_schema.sql
```

### 5. Start Redis

```bash
redis-server
```

### 6. Build and Run

```bash
go build -o server cmd/server/main.go
./server
```

The API will be available at `http://localhost:8080`

## 📚 API Documentation

### Base URL
```
http://localhost:8080/api/v1
```

### Core Endpoints

#### Plans
- `GET /plans/` - List all plans
- `POST /plans/` - Create new plan
- `GET /plans/{id}` - Get plan by ID
- `PUT /plans/{id}` - Update plan
- `DELETE /plans/{id}` - Delete plan
- `GET /plans/active` - Get active plans only
- `GET /plans/compare` - Compare multiple plans
- `GET /plans/{id}/analytics` - Get plan analytics

#### Subscriptions
- `GET /subscriptions/` - List subscriptions
- `POST /subscriptions/` - Create subscription
- `GET /subscriptions/{id}` - Get subscription by ID
- `PUT /subscriptions/{id}` - Update subscription
- `DELETE /subscriptions/{id}` - Cancel subscription

#### Health Check
- `GET /health` - System health status

### Example Usage

#### Create a Plan

```bash
curl -X POST http://localhost:8080/api/v1/plans/ \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Pro Plan",
    "description": "Professional plan with advanced features",
    "price": 19.99,
    "currency": "USD",
    "billing_cycle": "monthly",
    "features": {
      "feature1": true,
      "feature2": true,
      "max_storage": "100GB",
      "api_calls": 10000
    },
    "max_usage_per_day": 200,
    "max_usage_per_month": 6000,
    "is_active": true
  }'
```

#### List All Plans

```bash
curl -X GET http://localhost:8080/api/v1/plans/
```

#### Compare Plans

```bash
curl -X GET "http://localhost:8080/api/v1/plans/compare?plan_ids=uuid1&plan_ids=uuid2"
```

## 🧪 Testing

Run the test suite:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

## 📊 Monitoring

The application exposes Prometheus metrics at `/metrics` and provides health checks at `/health`.

## 🔧 Configuration

Configuration is managed through `configs/config.yaml`. Key configuration options:

- Database connection settings
- Redis connection settings
- Server port and host
- Logging levels
- Feature flags

## 🚀 Deployment

### Docker

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

### Environment Variables

- `DB_HOST` - Database host
- `DB_PORT` - Database port
- `DB_USER` - Database user
- `DB_PASSWORD` - Database password
- `DB_NAME` - Database name
- `REDIS_HOST` - Redis host
- `REDIS_PORT` - Redis port
- `SERVER_PORT` - Server port

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 📞 Support

For support and questions:
- Create an issue in the repository
- Check the [documentation](docs/payflow_lld.md)
- Review the [API documentation](docs/)

## 🔮 Roadmap

- [ ] Multi-tenancy support
- [ ] Advanced analytics with ML
- [ ] Payment gateway integrations
- [ ] Mobile SDK
- [ ] GraphQL API
- [ ] Event sourcing implementation
- [ ] Kubernetes deployment support

---

**PayFlow API** - Empowering digital content monetization with flexible, scalable subscription management. 
