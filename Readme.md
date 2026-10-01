# TaskLattice

Multi-tenant task-management backend in Go: organizations own projects,
projects own tasks, with RBAC, audit logs, a transactional outbox, SQS
workers, Redis rate limiting/caching, and idempotent write APIs.
Modular monolith: HTTP handlers stay thin, services hold business logic
and authorization, repositories hold parameterized PostgreSQL access.

## Contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [API Reference](#api-reference)
- [Key Behaviors](#key-behaviors)
- [Error Codes](#error-codes)
- [Operations](#operations)
- [Project Structure](#project-structure)
- [Verification](#verification)
- [Technologies Used](#technologies-used)

## Architecture

```mermaid
flowchart TB
    Client([HTTP clients]) --> API

    subgraph API ["API — cmd/api"]
        MW["Middleware<br/>request ID · auth · rate limit · idempotency"]
        H["Handlers<br/>bind input · render output"]
        S["Services<br/>validation · RBAC · transitions"]
        R["Repositories<br/>parameterized pgx · transactions"]
        MW --> H --> S --> R
    end

    R <--> PG[(PostgreSQL<br/>domain · audit · outbox)]
    S <--> Redis[(Redis<br/>rate limits · org cache)]
    API --> Client

    subgraph Worker ["Worker — cmd/worker"]
        PUB["Outbox publisher<br/>claim SKIP LOCKED · SQS send · mark"]
        CON["SQS consumer<br/>long poll · dedupe · dispatch"]
        CLN["Cleanup<br/>expired keys · stale sessions"]
    end

    PG --> PUB --> SQSQ["SQS queue"]
    SQSQ --> CON
    CON -->|max receives exceeded| DLQ["SQS dead-letter queue"]
    CON --> NTF["Notify handler"]
    CON --> REM["Due-date reminder handler"]
```

A write and its side effects commit atomically; everything downstream
is at-least-once with idempotent consumers:

```mermaid
sequenceDiagram
    participant C as Client
    participant A as API
    participant DB as PostgreSQL
    participant W as Worker publisher
    participant Q as SQS
    participant K as Worker consumer

    C->>A: POST /projects/:id/tasks + Idempotency-Key
    A->>A: auth · rate limit · replay check
    A->>DB: BEGIN → INSERT task → INSERT outbox → INSERT audit → COMMIT
    A-->>C: 201 task (version 1)
    W->>DB: claim unpublished (FOR UPDATE SKIP LOCKED)
    W->>Q: send event envelope
    W->>DB: mark published (or schedule retry with backoff)
    Q->>K: long-poll delivery
    K->>K: processed-events dedupe → handler
    K->>Q: delete (failures defer; DLQ via redrive policy)
```

Reads stay on PostgreSQL except short-lived organization caching in
Redis (60s TTL, invalidated on write, membership always rechecked).

## Prerequisites

- Go 1.26+
- PostgreSQL 14+
- Redis (optional; rate limiting degrades to in-memory, caching disables)
- SQS or a local emulator (ElasticMQ/LocalStack) for the worker
- `migrate` CLI for migrations (`scripts/migrate.sh` wraps it)

New Go dependencies (Redis, AWS SDK) are not vendored yet. Run once
where Go and network access exist:

```bash
go get github.com/redis/go-redis/v9@latest github.com/aws/aws-sdk-go-v2/config@latest github.com/aws/aws-sdk-go-v2/service/sqs@latest && go mod tidy
```

## Getting Started

### 1. Set Up PostgreSQL

```sql
CREATE DATABASE tasklattice;
```

### 2. Configure Environment Variables

Copy `.env.example` to `.env` and adjust:

```env
DATABASE_URL=postgres://username:password@localhost:5432/tasklattice?sslmode=disable
PORT=5000
JWT_SECRET=your-secure-jwt-secret-key
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h
REDIS_URL=redis://localhost:6379/0
RATE_LIMIT_AUTH_PER_MIN=5
RATE_LIMIT_API_PER_MIN=100
IDEMPOTENCY_TTL=24h
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=
SQS_QUEUE_URL=
SQS_DLQ_URL=
```

`REDIS_URL` empty means single-instance limiting and no caching.
`AWS_ENDPOINT_URL` points the worker at a local SQS emulator.
The worker additionally requires `SQS_QUEUE_URL` and `SQS_DLQ_URL`.

### 3. Run Database Migrations

```bash
./scripts/migrate.sh up
```

Migrations are append-only (`migrations/000001`–`000008`); roll back with
`./scripts/migrate.sh down [count]`.

### 4. Start the API

```bash
go run ./cmd/api
```

Or with Air for hot reloading:

```bash
air
```

The API listens on `PORT` (default `5000`) under `/api/v1`.

### 5. Start the Worker

```bash
go run ./cmd/worker
```

The worker relays outbox events to SQS, consumes them (notifications,
due-date reminders), and sweeps expired transient rows. It exits fast
when `SQS_QUEUE_URL`/`SQS_DLQ_URL` are unset.

## API Reference

All domain routes require `Authorization: Bearer <access-token>`.
Errors use `{"error": {"code", "message", "request_id"}}`.

### Auth

```http
POST /api/v1/auth/register   {email, password}
POST /api/v1/auth/login      {email, password}  -> user + access/refresh tokens
POST /api/v1/auth/refresh    {refresh_token}    -> rotated pair (reuse revokes all sessions)
POST /api/v1/auth/logout     {refresh_token}
POST /api/v1/auth/logout-all (authenticated)
```

### Organizations (multi-tenant boundary)

```http
POST   /api/v1/organizations
GET    /api/v1/organizations
GET    /api/v1/organizations/:organizationID
PATCH  /api/v1/organizations/:organizationID            (admin)
DELETE /api/v1/organizations/:organizationID            (owner, soft delete)
GET    /api/v1/organizations/:organizationID/audit-logs (admin, ?limit=&offset=)
```

### Members (roles: OWNER > ADMIN > MEMBER > VIEWER)

```http
POST   /api/v1/organizations/:organizationID/members   {user_id, role} (admin; owner grants admin/owner)
GET    /api/v1/organizations/:organizationID/members
PATCH  /api/v1/organizations/:organizationID/members/:userID {role}
DELETE /api/v1/organizations/:organizationID/members/:userID
```

The final owner cannot be demoted or removed.

### Projects

```http
POST   /api/v1/organizations/:organizationID/projects  (admin)
GET    /api/v1/organizations/:organizationID/projects
GET    /api/v1/projects/:projectID
PATCH  /api/v1/projects/:projectID                     (admin)
DELETE /api/v1/projects/:projectID                     (admin, soft delete)
```

### Tasks (member write, anyone reads)

```http
POST /api/v1/projects/:projectID/tasks   {title, description?, status?, priority?, assignee_id?, due_date?}
GET  /api/v1/projects/:projectID/tasks   (filters + cursor pagination, see below)
GET  /api/v1/tasks/:taskID               (includes labels)
PATCH /api/v1/tasks/:taskID              (requires version; see concurrency)
DELETE /api/v1/tasks/:taskID             (soft delete)
POST /api/v1/tasks/:taskID/restore
```

List query params: `status`, `priority`, `assignee_id`, `creator_id`,
`due_before`, `due_after`, `created_before`, `created_after` (RFC3339),
`label`, `search` (PostgreSQL full text over title/description),
`sort` (`created_at`/`updated_at`/`due_date`), `order` (`asc`/`desc`),
`limit` (default 20, max 100), `cursor` (opaque, sort-bound).

```json
{ "data": [], "pagination": { "next_cursor": "...", "has_more": true } }
```

### Comments (member write; author or admin edits/deletes)

```http
POST   /api/v1/tasks/:taskID/comments   {body}
GET    /api/v1/tasks/:taskID/comments
PATCH  /api/v1/comments/:commentID      {body}
DELETE /api/v1/comments/:commentID      (soft delete)
```

### Labels (member creates/attaches; admin renames/deletes)

```http
POST   /api/v1/projects/:projectID/labels   {name}
GET    /api/v1/projects/:projectID/labels
PATCH  /api/v1/labels/:labelID               {name}
DELETE /api/v1/labels/:labelID
POST   /api/v1/tasks/:taskID/labels          {label_id}
DELETE /api/v1/tasks/:taskID/labels/:labelID
```

### Health

```http
GET /health/live    (process alive)
GET /health/ready   (postgres + redis checks)
```

## Key Behaviors

- **RBAC:** every protected operation verifies membership and role
  server-side; nested resources re-resolve their organization, and task
  assignees must be organization members.
- **Optimistic concurrency:** `PATCH /tasks/:taskID` requires the current
  `version`; a stale version returns `409 RESOURCE_VERSION_CONFLICT`.
- **Idempotency:** send `Idempotency-Key` with POST creates; the original
  response replays for 24h, key reuse with a different body is rejected,
  and server errors release the key. Assignee/due-date clearing uses
  explicit `clear_assignee` / `clear_due_date` flags.
- **Rate limiting:** auth endpoints 5/min/IP, API 100/min/user
  (configurable), distributed over Redis with in-memory fallback.
- **Reliability:** mutations commit outbox events and audit rows in the
  same transaction; the publisher relays them to SQS with capped backoff,
  consumers are idempotent via processed-event dedupe, poison messages
  reach the DLQ through the redrive policy, and the worker shuts down
  gracefully (drains in-flight work, honors a shutdown timeout).
- **Caching:** organization reads cache for 60s in Redis with
  write-through invalidation; membership is always rechecked.

## Error Codes

Stable `error.code` values returned in the error envelope:

| Code | Meaning |
| ---- | ------- |
| `VALIDATION_ERROR` | Invalid input (also bad UUIDs, dates, sort/limit) |
| `UNAUTHORIZED` | Missing or invalid credentials/token |
| `FORBIDDEN` | Authenticated but not a member, or role too weak |
| `NOT_FOUND` | Resource missing, soft-deleted, or in another org |
| `CONFLICT` | State conflict (duplicate member/label, active restore, request in progress) |
| `RESOURCE_VERSION_CONFLICT` | Stale task `version` on update |
| `IDEMPOTENCY_KEY_REUSED` | Key reused with a different request body |
| `EMAIL_ALREADY_REGISTERED` | Duplicate registration |
| `INVALID_CREDENTIALS` | Bad email/password (never distinguishes which) |
| `TOKEN_EXPIRED` / `INVALID_TOKEN` | Refresh/access token problems |
| `REFRESH_TOKEN_REUSED` | Revoked refresh token presented; all sessions revoked |
| `RATE_LIMITED` | Over limit; honours `Retry-After` |
| `INTERNAL_ERROR` | Server failure; no internals leak |

## Operations

- **Migrations:** append-only, every change ships `up` + `down`.
  Apply with `./scripts/migrate.sh up`, roll back with
  `./scripts/migrate.sh down [count]`.
- **Startup:** the API fails fast on bad config (`DATABASE_URL`,
  `JWT_SECRET`); Redis degrades gracefully; the worker refuses to start
  without `SQS_QUEUE_URL`/`SQS_DLQ_URL`.
- **Shutdown:** both processes stop accepting work on SIGTERM/SIGINT,
  drain in-flight requests and messages, then close Redis/PostgreSQL
  within `SHUTDOWN_TIMEOUT`.
- **Queues:** the worker creates the queue, DLQ, and redrive policy
  (`SQS_MAX_RECEIVE_COUNT`) on startup; poison messages age into the DLQ
  while the outbox publisher retries delivery with capped backoff.
- **Monitoring hooks:** `/health/live` for liveness, `/health/ready`
  for postgres/redis readiness, structured `request_id` on every error,
  and `Retry-After` on `429`.

## Project Structure

```
tasklattice/
├── cmd/
│   ├── api/                     # API entrypoint, route wiring
│   └── worker/                  # Background worker entrypoint
├── internal/
│   ├── config/                  # Environment configuration
│   ├── database/                # pgx pool setup
│   ├── handlers/                # Thin HTTP handlers (auth, orgs, projects, tasks, comments, labels, audit, health)
│   ├── middleware/              # Auth, request ID, in-memory + Redis rate limiting, idempotency
│   ├── models/                  # Domain types, roles, task query/page, events/audit
│   ├── repository/              # Parameterized pgx persistence, outbox/audit/idempotency, DBTX
│   ├── services/                # Business logic + server-side authorization
│   └── worker/                  # SQS client, outbox publisher, consumer, notify/reminder/cleanup
├── migrations/                  # Append-only up/down migrations (000001–000008)
├── scripts/
│   └── migrate.sh               # Migration helper
├── .air.toml                    # Air configuration
├── .env.example                 # Environment template
├── go.mod
└── go.sum
```

Legacy `/api/v1/todos` routes remain for backward compatibility.

## Verification

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
```

## Technologies Used

- **Go 1.26+**: Backend programming language
- **Gin**: HTTP web framework
- **PostgreSQL + pgx/v5**: Relational store, driver and pool
- **Redis (go-redis/v9)**: Distributed rate limiting, short-lived cache
- **AWS SQS (aws-sdk-go-v2)**: Async event transport with DLQ redrive
- **JWT + bcrypt**: Access/refresh sessions, password hashing
- **golang-migrate**: Database migrations
- **Air**: Hot reloading for development
- **godotenv**: Environment variable management
