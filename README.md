# Seq

A trading backend system responsible for event handling, market data, risk management, and API connectivity.

## Overview

Seq is a high-performance Go-based trading system backend that provides essential services for trading operations including portfolio management, event management, secret management, and instrument catalog services.

## Features

- **Portfolio Management System (PMS)**: Manages trading portfolios and instrument catalogs
- **Event Management System (EMS)**: Handles trading events and order processing
- **Secret Management System (SMS)**: Securely manages API keys and credentials
- **Instrument Catalog**: Provides access to trading instruments and market data
- **Structured Logging**: Comprehensive logging with rotation support (stdout or file)
- **Database Migrations**: Version-controlled database schema management
- **PostgreSQL Integration**: Uses GORM ORM with PostgreSQL for data persistence
- **Configuration Management**: YAML-based configuration with environment variable support

## Architecture

### Services

- **PMS (Portfolio Management System)**: Manages portfolios and instruments
- **EMS (Event Management System)**: Handles trading events and order flow
- **SMS (Secret Management System)**: Manages API credentials and secrets
- **Catalog**: Provides instrument catalog services

### Key Components

- **Logger**: Singleton logger with support for stdout/file output and log rotation
- **Database**: PostgreSQL connection pool with GORM
- **Config**: YAML-based configuration system
- **Migrations**: Database schema versioning and management

## Getting Started

### Prerequisites

- Go 1.25.1 or later
- PostgreSQL 16 or later
- Make (for build automation)

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd seq
```

2. Install dependencies:
```bash
make deps
# or
go mod download
```

3. Set up PostgreSQL (using Docker):
```bash
docker-compose up -d
```

4. Configure the application:
```bash
cp config/local.yml config/myconfig.yml
# Edit config/myconfig.yml with your database credentials
```

5. Run database migrations:
```bash
make migrate CONFIG=config/myconfig.yml
```

6. Build and run:
```bash
make run CONFIG=config/myconfig.yml
# or
./bin/seq -c config/myconfig.yml
```

## Configuration

Configuration is managed through YAML files. The system supports two ways to specify the config file:

1. Command-line flag: `./bin/seq -c config/prod.yml`
2. Environment variable: `CONFIG=./config/prod.yml ./bin/seq`

### Configuration File Structure

```yaml
logger:
  level: debug              # trace, debug, info, warn, error, fatal, panic
  output: stdout            # "stdout" or "file"
  path: logs/seq.log        # Required when output is "file"
  max_byte_size: 10485760   # Max file size in bytes before rotation (0 = no rotation)
  max_backup_files: 5       # Max number of backup files to keep (0 = keep all)

ems:
  url: http://localhost:8080

pms:
  url: http://localhost:8081
  database:
    host: localhost
    port: 5432
    user: postgres
    password: postgres
    dbname: seq
    sslmode: disable        # disable, allow, prefer, require, verify-ca, verify-full
```

### Logger Configuration

- **level**: Logging level (trace, debug, info, warn, error, fatal, panic)
- **output**: Output destination - `stdout` for console output or `file` for file output
- **path**: Log file path (required when `output` is `file`)
- **max_byte_size**: Maximum log file size in bytes before rotation. Set to `0` to disable rotation
- **max_backup_files**: Maximum number of rotated log files to keep. Set to `0` to keep all backups

### Database Configuration

- **host**: PostgreSQL server hostname
- **port**: PostgreSQL server port (default: 5432)
- **user**: Database username
- **password**: Database password
- **dbname**: Database name
- **sslmode**: SSL connection mode (disable, allow, prefer, require, verify-ca, verify-full)

## Database Migrations

### Overview

Seq uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema versioning and management. Migrations are stored in the `migrations/` directory as SQL files following the naming convention:

- `{version}_{description}.up.sql` - Migration to apply
- `{version}_{description}.down.sql` - Migration to rollback

### Running Migrations

#### Using Make (Recommended)

```bash
# Run migrations with default config (config/local.yml)
make migrate

# Run migrations with custom config
make migrate CONFIG=config/prod.yml
```

#### Using Go Run

```bash
# Using default config
go run cmd/migrate/main.go -c config/local.yml

# Using environment variable
CONFIG=config/prod.yml go run cmd/migrate/main.go -c config/prod.yml
```

### Migration File Naming

Migration files must follow this naming convention:
- Format: `{version}_{description}.{direction}.sql`
- Version: Sequential number (e.g., `000001`, `000002`)
- Description: Descriptive name with underscores (e.g., `create_instrument`, `add_user_table`)
- Direction: `up` or `down`

Examples:
- `000001_create_instrument.up.sql`
- `000001_create_instrument.down.sql`
- `000002_add_index.up.sql`
- `000002_add_index.down.sql`

### Creating New Migrations

1. Create a new migration file pair in the `migrations/` directory:
```bash
# Example: Creating migration 000002
touch migrations/000002_add_user_table.up.sql
touch migrations/000002_add_user_table.down.sql
```

2. Write your SQL in the `.up.sql` file:
```sql
-- migrations/000002_add_user_table.up.sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

3. Write the reverse operation in the `.down.sql` file:
```sql
-- migrations/000002_add_user_table.down.sql
DROP TABLE users;
```

4. Run the migration:
```bash
make migrate CONFIG=config/local.yml
```

### Migration Best Practices

- **Always create both up and down migrations**: This ensures you can rollback if needed
- **Use sequential version numbers**: Migrations are applied in version order
- **Keep migrations small and focused**: One logical change per migration
- **Test migrations**: Test both up and down migrations before deploying
- **Never modify existing migrations**: If you need to change a migration, create a new one
- **Use transactions carefully**: Some DDL statements in PostgreSQL cannot be rolled back
- **Backup before migrations**: Always backup your database before running migrations in production

### Migration State

The migration tool automatically tracks which migrations have been applied in the database using a `schema_migrations` table. This table is created automatically on the first migration run.

### Troubleshooting Migrations

- **Migration already applied**: If a migration fails partway through, you may need to manually fix the database state or use `migrate force` (not included in this tool)
- **Connection errors**: Ensure your database configuration in the config file is correct
- **Permission errors**: Ensure the database user has CREATE TABLE and other necessary permissions
- **Migration order**: Migrations are applied in version order - ensure your version numbers are sequential

## Development

### Building

```bash
# Build for local platform
make build-local

# Build for Linux AMD64
make build

# Build everything
go build ./...
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run benchmarks
make benchmark
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run go vet
make vet
```

### Available Make Targets

- `make` or `make all` - Build and run all tests (default)
- `make build` - Build for linux/amd64
- `make build-local` - Build for local platform
- `make run` - Build and run the application
- `make test` - Run tests
- `make test-coverage` - Run tests with coverage report
- `make benchmark` - Run benchmarks
- `make lint` - Run golangci-lint
- `make migrate` - Run database migrations
- `make clean` - Remove build artifacts
- `make deps` - Download and tidy dependencies
- `make fmt` - Format code
- `make vet` - Run go vet
- `make help` - Show all available targets

## Project Structure

```
seq/
├── cmd/
│   ├── main.go              # Main application entry point
│   └── migrate/
│       └── main.go          # Migration tool entry point
├── config/
│   └── local.yml            # Example configuration file
├── internal/
│   ├── config/              # Configuration management
│   ├── db/                  # Database connection utilities
│   └── srv/                 # Service implementations
│       ├── catalog/         # Instrument catalog service (PMS)
│       ├── ems/             # Event management service
│       └── sms/             # Secret management service
├── migrations/              # Database migration files
│   ├── 000001_create_instrument.up.sql
│   └── 000001_create_instrument.down.sql
├── pkg/
│   └── logger/              # Logging package
├── bin/                     # Build output directory
├── logs/                    # Log files (if file logging enabled)
├── docker-compose.yml       # Docker Compose configuration
├── Makefile                 # Build automation
└── README.md                # This file
```

## Logging

The logger is a singleton that can be accessed from anywhere in the codebase:

```go
import "github.com/BullionBear/seq/pkg/logger"

// Get the singleton logger
log := logger.Get()

// Use it
log.Info().Msg("Application started")
log.Error().Err(err).Msg("Failed to process request")
log.Debug().Str("key", "value").Int("count", 42).Msg("Debug message")
```

Logger features:
- Singleton pattern - initialized once, accessible everywhere
- Support for stdout (human-readable) or file (JSON) output
- Automatic log rotation based on file size
- Configurable log levels
- Structured logging with fields

## Database

The system uses PostgreSQL with GORM as the ORM. Database connections are managed through the `internal/db` package and configured via the config file.

Connection pooling:
- Max idle connections: 10
- Max open connections: 100

## License

See LICENSE file for details.
