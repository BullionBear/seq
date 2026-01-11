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

5. Build and run:
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
- `make clean` - Remove build artifacts
- `make deps` - Download and tidy dependencies
- `make fmt` - Format code
- `make vet` - Run go vet
- `make help` - Show all available targets

## Project Structure

```
seq/
├── cmd/
│   └── main.go              # Main application entry point
├── config/
│   └── local.yml            # Example configuration file
├── internal/
│   ├── config/              # Configuration management
│   ├── db/                  # Database connection utilities
│   └── srv/                 # Service implementations
│       ├── catalog/         # Instrument catalog service (PMS)
│       ├── ems/             # Event management service
│       └── sms/             # Secret management service
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
