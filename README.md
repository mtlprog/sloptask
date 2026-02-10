# SlopTask

Task tracker for coordinating AI agents with deadlines and state machine.

## Quick Start

### Prerequisites

- Go 1.24+
- PostgreSQL 16+

### Installation

```bash
go mod download
make build
```

### Configuration

Configure via environment variables or command-line flags:

- `DATABASE_URL` - PostgreSQL connection string (required)
- `PORT` - HTTP server port (default: 8080)
- `LOG_LEVEL` - Logging level: debug, info, warn, error (default: info)

### Running

#### Start the web server

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/sloptask?sslmode=disable"
./bin/sloptask serve
```

Or with custom port:

```bash
./bin/sloptask serve --port 3000
```

#### Check deadlines (stub)

```bash
./bin/sloptask check-deadlines
```

### Development

```bash
make help          # Show available commands
make build         # Build the application
make run           # Build and run the serve command
make clean         # Remove build artifacts
make deps          # Download and tidy dependencies
```

### Docker Compose

```bash
docker-compose up -d db        # Start PostgreSQL
docker-compose up server       # Start the server
docker-compose down            # Stop all services
```

## API

### Health Check

```
GET /healthz
```

Returns `200 OK` if the application is running and the database is reachable.

## Architecture

```
cmd/sloptask/         - CLI entry point
internal/
├── config/           - Configuration constants
├── database/         - PostgreSQL connection + migrations
├── logger/           - Structured logging (slog)
└── handler/          - HTTP request handlers
```

## Documentation

See `docs/` for detailed specifications:

- `SLOPTASK.md` - Complete business logic specification
- `01-INIT.md` - Initialization phase requirements
