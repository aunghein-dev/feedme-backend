# feedme-backend

`feedme-backend` is a Go HTTP service for RSS aggregation. It provides a small API for user registration, feed registration, feed subscriptions, and personalized post retrieval, while a background scraper continuously ingests RSS items into PostgreSQL.

This project follows a pragmatic Go service shape:

- a thin HTTP transport layer built with `chi`
- API key authentication via middleware
- SQL-first persistence with `sqlc`
- schema evolution through `goose` migrations
- background ingestion running inside the service process

## Architecture

The service is intentionally simple and modular:

- `main.go` boots configuration, database connectivity, the background scraper, and the HTTP router
- `handler_*.go` contains transport-facing request handlers
- `middelware_auth.go` resolves the authenticated user from the `Authorization` header
- `scraper.go` schedules concurrent feed pulls and persists normalized posts
- `rss.go` fetches and unmarshals RSS XML
- `internal/database` contains `sqlc`-generated query code and models
- `sql/schema` contains ordered PostgreSQL migrations
- `sql/queries` contains the source SQL contracts used by `sqlc`

At runtime, the application behaves as two cooperating subsystems:

1. The API server accepts authenticated CRUD-style requests for users, feeds, follows, and posts.
2. The scraper goroutine polls registered feeds every minute using 10 workers, parses RSS items, and persists posts with duplicate protection at the database layer.

## Core Capabilities

- Create users with generated API keys
- Register RSS feeds
- Follow and unfollow feeds per user
- Fetch a user-specific post timeline from followed feeds
- Continuously ingest RSS content in the background
- Avoid duplicate feeds and duplicate posts with database constraints

## Tech Stack

- Go 1.26+
- PostgreSQL
- `chi` for HTTP routing
- `cors` for permissive cross-origin access during development
- `sqlc` for type-safe query generation
- `goose` for schema migrations

## Data Model

The database is centered around four tables:

- `users`: service users with generated API keys
- `feeds`: RSS feed sources registered by users
- `feed_follows`: join table connecting users to feeds
- `posts`: normalized feed items linked back to a source feed

The scraper selects feeds ordered by `last_fetched_at`, fetches the least recently scraped entries first, and updates that timestamp after each scrape cycle.

## Configuration

The service reads configuration from environment variables, with `.env` support via `godotenv`.

| Variable | Required | Description |
| --- | --- | --- |
| `PORT` | yes | HTTP port the server will bind to |
| `DB_URL` | yes | PostgreSQL connection string |

Example `.env`:

```env
PORT=9000
DB_URL=postgres://postgres:postgres@localhost:5432/feedme?sslmode=disable
```

## Local Development

### 1. Install dependencies

You need:

- Go
- PostgreSQL
- `goose` for migrations
- `sqlc` if you want to regenerate database code after changing SQL

### 2. Create the database

Create a PostgreSQL database and update `DB_URL` to point at it.

### 3. Run migrations

```bash
goose -dir sql/schema postgres "$DB_URL" up
```

### 4. Regenerate query code when SQL changes

```bash
sqlc generate
```

### 5. Start the service

```bash
go build
./feedme-backend
```

When the service starts, it will:

- open the PostgreSQL connection
- launch the RSS scraper in a background goroutine
- expose the HTTP API on `:$PORT`

## API Overview

Base path: `/v1`

Authentication for protected endpoints uses:

```http
Authorization: ApiKey <your_api_key>
```

### Public Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/v1/healthz` | Readiness check |
| `GET` | `/v1/error` | Error response example |
| `POST` | `/v1/users` | Create a user and issue an API key |

### Authenticated Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/v1/users` | Get the current authenticated user |
| `GET` | `/v1/posts` | Get up to 10 posts for the current user |
| `POST` | `/v1/feeds` | Register a new feed |
| `GET` | `/v1/feeds` | List registered feeds |
| `POST` | `/v1/feed-follows` | Follow a feed |
| `GET` | `/v1/feed-follows` | List the current user's follows |
| `DELETE` | `/v1/feed-follows/{feedFollowID}` | Remove a follow |

## Example Workflow

### Create a user

```bash
curl -X POST http://localhost:9000/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Aung Hein"}'
```

Example response:

```json
{
  "id": "3f6c6fd2-6c83-4c17-b0d1-7e9dd4f4d16c",
  "created_at": "2026-03-29T12:00:00Z",
  "updated_at": "2026-03-29T12:00:00Z",
  "name": "Aung Hein",
  "api_key": "generated-api-key"
}
```

### Register a feed

```bash
curl -X POST http://localhost:9000/v1/feeds \
  -H "Content-Type: application/json" \
  -H "Authorization: ApiKey <your_api_key>" \
  -d '{
    "name": "Boot.dev Blog",
    "url": "https://blog.boot.dev/index.xml"
  }'
```

### Follow a feed

```bash
curl -X POST http://localhost:9000/v1/feed-follows \
  -H "Content-Type: application/json" \
  -H "Authorization: ApiKey <your_api_key>" \
  -d '{
    "feed_id": "feed-uuid-here"
  }'
```

### Fetch your personalized posts

```bash
curl http://localhost:9000/v1/posts \
  -H "Authorization: ApiKey <your_api_key>"
```

## Scraper Behavior

The background scraper is currently configured in code to:

- run every 1 minute
- use 10 concurrent workers
- select feeds by oldest `last_fetched_at`
- parse common RSS publication date formats
- persist posts with UTC timestamps
- skip duplicate post URLs gracefully

This keeps the service easy to reason about during development, while leaving room for future extraction into a dedicated worker process if scale requires it.

## Project Layout

```text
.
├── main.go
├── scraper.go
├── rss.go
├── handler_*.go
├── middelware_auth.go
├── internal/
│   ├── auth/
│   └── database/
├── sql/
│   ├── queries/
│   └── schema/
├── sqlc.yaml
└── README.md
```

## Engineering Notes

From a production-readiness perspective, the next improvements I would prioritize are:

- make scraper interval and concurrency configurable
- add pagination support for posts
- add structured logging and request tracing
- add HTTP and scraper-level timeouts, retries, and backoff
- introduce integration tests for handlers and scraper workflows
- tighten CORS policy for non-development environments

## Development Workflow Summary

```bash
goose -dir sql/schema postgres "$DB_URL" up
sqlc generate
go build
./feedme-backend
```

For day-to-day API development, the simplest bootstrap path is:

1. create a user
2. save the returned API key
3. create a feed
4. follow the feed
5. wait for the scraper cycle
6. fetch posts
