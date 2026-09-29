# TaskFlow --- Production-Grade Todo API Extension

## 1. Project Overview

TaskFlow is an extension of the existing Go Todo API into a
production-oriented, multi-tenant task management backend.

The goal of Phase 1 is **not** to build a huge Todo application or
introduce unnecessary infrastructure.

The goal is to build a strong backend foundation that demonstrates:

-   Clean Go architecture
-   Multi-tenancy
-   Authentication and authorization
-   RBAC
-   PostgreSQL data modeling
-   Transactions
-   Cursor-based pagination
-   Optimistic concurrency
-   Idempotent APIs
-   Transactional outbox
-   Background processing
-   Redis
-   AWS SQS
-   Reliable failure handling
-   API design and documentation

The application should remain a **modular monolith** in Phase 1.

------------------------------------------------------------------------

# 2. Product Vision

Transform the current Todo CRUD API into a small collaborative
task-management platform similar in concept to a simplified
Linear/Jira/Asana backend.

The system will support:

``` text
User
 |
 +-- Organization
       |
       +-- Members / Roles
       |
       +-- Projects
              |
              +-- Tasks
                    |
                    +-- Comments
                    +-- Labels
                    +-- Activity
```

The backend should be designed so that it can later evolve into a larger
distributed system without requiring a complete rewrite.

------------------------------------------------------------------------

# 3. Phase 1 Scope

Phase 1 includes:

-   API architecture refactoring
-   Authentication
-   Refresh-token sessions
-   Organizations
-   Organization membership
-   RBAC
-   Projects
-   Tasks
-   Comments
-   Labels
-   Soft deletion
-   Cursor pagination
-   Filtering and sorting
-   Search using PostgreSQL
-   Optimistic concurrency
-   Transactions
-   Audit logs
-   Transactional outbox
-   AWS SQS
-   Background workers
-   Retry handling
-   Dead-letter queue
-   Idempotency
-   Redis
-   Distributed rate limiting
-   API versioning
-   Centralized error handling
-   Graceful shutdown
-   Health/readiness endpoints
-   API documentation

------------------------------------------------------------------------

# 4. Phase 2 Scope

The following are intentionally postponed to Phase 2:

-   CI/CD
-   Automated testing strategy and full test suite
-   Containerized deployment
-   Docker production setup
-   Kubernetes
-   Prometheus
-   Grafana
-   Loki
-   Tempo
-   OpenTelemetry
-   Distributed tracing
-   Production dashboards
-   Production alerts
-   Load-testing infrastructure
-   Container security scanning
-   Automated deployment pipelines

Phase 2 will focus on turning the completed Phase 1 application into a
fully operational production system.

------------------------------------------------------------------------

# 5. Non-Goals

The following are not part of the initial implementation:

-   Full Jira/Linear feature parity
-   Complex frontend
-   Native mobile applications
-   Microservices architecture
-   GraphQL
-   Multiple databases for the same domain
-   Elasticsearch/OpenSearch
-   Billing/subscriptions
-   Enterprise SSO/SAML
-   Kubernetes
-   Complex analytics platform

The project should remain intentionally focused.

------------------------------------------------------------------------

# 6. Target Users

## Organization Owner

Can:

-   Create organizations
-   Manage members
-   Change roles
-   Create projects
-   Manage all tasks
-   View activity/audit history

## Organization Admin

Can:

-   Manage projects
-   Manage members where permitted
-   Manage tasks
-   View organization activity

## Member

Can:

-   View projects
-   Create tasks
-   Update permitted tasks
-   Assign tasks
-   Add comments
-   Manage labels where permitted

## Viewer

Can:

-   View organizations
-   View projects
-   View tasks
-   View comments

Cannot modify resources.

------------------------------------------------------------------------

# 7. Architecture

Phase 1 should use a modular monolith.

``` text
                         Client
                           |
                           v
                    +-------------+
                    |   Go / Gin  |
                    |     API     |
                    +------+------+ 
                           |
          +----------------+----------------+
          |                |                |
          v                v                v
        Auth          Task Service     Project Service
          |                |                |
          +----------------+----------------+
                           |
                           v
                    +-------------+
                    | PostgreSQL  |
                    +------+------+
                           |
                           v
                    Transactional
                       Outbox
                           |
                           v
                    Outbox Publisher
                           |
                           v
                         SQS
                           |
             +-------------+-------------+
             |             |             |
             v             v             v
        Notification    Reminder      Other
          Worker         Worker       Workers

                    +-------------+
                    |    Redis    |
                    +-------------+
```

The core dependency direction should be:

``` text
HTTP Handler
     |
     v
Service / Use Case
     |
     v
Repository
     |
     v
PostgreSQL
```

Handlers should not contain significant business logic.

------------------------------------------------------------------------

# 8. Recommended Project Structure

For now, keep the entire project in one repository and use
sections/modules rather than splitting the system into multiple
repositories.

``` text
todo-api/
|
├── cmd/
│   ├── api/
│   ├── worker/
│   └── migrate/
|
├── internal/
│   ├── auth/
│   ├── organization/
│   ├── project/
│   ├── task/
│   ├── comment/
│   ├── label/
│   ├── audit/
│   ├── notification/
│   ├── outbox/
│   ├── worker/
│   ├── middleware/
│   ├── database/
│   ├── config/
│   └── errors/
|
├── migrations/
|
├── README.md
├── Makefile
├── .env.example
├── openapi.yaml
└── go.mod
```

Additional documentation files are not required during Phase 1.

------------------------------------------------------------------------

# 9. Authentication

## 9.1 Registration

``` http
POST /api/v1/auth/register
```

Requirements:

-   Validate email.
-   Normalize email.
-   Hash password securely.
-   Prevent duplicate users.
-   Never return password hashes.
-   Never log passwords.

## 9.2 Login

``` http
POST /api/v1/auth/login
```

The API should issue:

``` text
Access Token
    |
    +-- short lifetime

Refresh Token
    |
    +-- longer lifetime
```

Access tokens should be short-lived.

Refresh tokens should be persisted securely as hashes rather than
storing raw tokens.

## 9.3 Refresh

``` http
POST /api/v1/auth/refresh
```

Requirements:

-   Validate refresh token.
-   Rotate refresh token.
-   Revoke previous token.
-   Issue new access token.
-   Detect refresh-token reuse where practical.

## 9.4 Logout

``` http
POST /api/v1/auth/logout
```

Revoke the current refresh session.

## 9.5 Logout All

``` http
POST /api/v1/auth/logout-all
```

Revoke all refresh sessions for the authenticated user.

------------------------------------------------------------------------

# 10. Organizations

Organizations provide the multi-tenant boundary.

## Create

``` http
POST /api/v1/organizations
```

The authenticated user becomes the owner.

## List

``` http
GET /api/v1/organizations
```

Return organizations where the user is a member.

## Get

``` http
GET /api/v1/organizations/:organizationID
```

## Update

``` http
PATCH /api/v1/organizations/:organizationID
```

Owner/admin authorization required.

## Delete

``` http
DELETE /api/v1/organizations/:organizationID
```

Owner authorization required.

------------------------------------------------------------------------

# 11. Organization Membership

Endpoints:

``` http
POST   /api/v1/organizations/:organizationID/members
GET    /api/v1/organizations/:organizationID/members
PATCH  /api/v1/organizations/:organizationID/members/:userID
DELETE /api/v1/organizations/:organizationID/members/:userID
```

Supported roles:

``` text
OWNER
ADMIN
MEMBER
VIEWER
```

Requirements:

-   Verify organization membership on every protected operation.
-   Prevent unauthorized role changes.
-   Prevent removal of the final organization owner.
-   Prevent cross-organization resource access.
-   Enforce authorization server-side.

------------------------------------------------------------------------

# 12. Projects

A project belongs to an organization.

Endpoints:

``` http
POST   /api/v1/organizations/:organizationID/projects
GET    /api/v1/organizations/:organizationID/projects
GET    /api/v1/projects/:projectID
PATCH  /api/v1/projects/:projectID
DELETE /api/v1/projects/:projectID
```

Project fields:

``` text
id
organization_id
name
description
created_by
created_at
updated_at
deleted_at
```

------------------------------------------------------------------------

# 13. Tasks

Tasks are the primary domain resource.

## Task Fields

``` text
id
organization_id
project_id
title
description
status
priority
creator_id
assignee_id
due_date
version
created_at
updated_at
deleted_at
```

## Status

``` text
TODO
IN_PROGRESS
BLOCKED
DONE
CANCELLED
```

## Priority

``` text
LOW
MEDIUM
HIGH
URGENT
```

## Create

``` http
POST /api/v1/projects/:projectID/tasks
```

## Get

``` http
GET /api/v1/tasks/:taskID
```

## Update

``` http
PATCH /api/v1/tasks/:taskID
```

## Delete

``` http
DELETE /api/v1/tasks/:taskID
```

Deletion should use soft deletion.

## Restore

``` http
POST /api/v1/tasks/:taskID/restore
```

------------------------------------------------------------------------

# 14. Task Listing and Pagination

The API must support scalable task retrieval.

Example:

``` http
GET /api/v1/projects/:projectID/tasks?
    status=IN_PROGRESS&
    priority=HIGH&
    assignee_id=<uuid>&
    limit=20&
    cursor=<cursor>&
    sort=created_at&
    order=desc
```

Supported filters:

-   status
-   priority
-   assignee
-   creator
-   due date
-   created date
-   label
-   search

Use cursor-based pagination.

Example:

``` json
{
  "data": [],
  "pagination": {
    "next_cursor": "opaque-cursor",
    "has_more": true
  }
}
```

Avoid returning unbounded collections.

------------------------------------------------------------------------

# 15. PostgreSQL Search

Initial task search should use PostgreSQL.

Example:

``` http
GET /api/v1/projects/:projectID/tasks?search=payment
```

Search should initially cover fields such as:

-   title
-   description

A dedicated search engine should not be introduced unless PostgreSQL
search becomes insufficient.

------------------------------------------------------------------------

# 16. Optimistic Concurrency

Tasks must contain a version number.

Example:

``` text
Task ID: 123
Version: 7
```

Updates must include the expected version.

Conceptually:

``` sql
UPDATE tasks
SET
    title = $1,
    version = version + 1
WHERE
    id = $2
    AND version = $3;
```

If no row is updated:

``` http
409 Conflict
```

Response:

``` json
{
  "error": {
    "code": "RESOURCE_VERSION_CONFLICT",
    "message": "The task was modified by another request"
  }
}
```

This prevents silent lost updates.

------------------------------------------------------------------------

# 17. Comments

Endpoints:

``` http
POST   /api/v1/tasks/:taskID/comments
GET    /api/v1/tasks/:taskID/comments
PATCH  /api/v1/comments/:commentID
DELETE /api/v1/comments/:commentID
```

Requirements:

-   User must have access to the task.
-   Author permissions must be enforced.
-   Comment changes should create activity/audit events where
    appropriate.

------------------------------------------------------------------------

# 18. Labels

Projects may contain labels.

Examples:

``` text
bug
backend
frontend
security
urgent
customer-request
```

Endpoints:

``` http
POST   /api/v1/projects/:projectID/labels
GET    /api/v1/projects/:projectID/labels
PATCH  /api/v1/labels/:labelID
DELETE /api/v1/labels/:labelID
```

A task can have multiple labels.

------------------------------------------------------------------------

# 19. Audit Logs

Important mutations should create audit records.

Examples:

``` text
organization.created
member.added
member.role_changed
project.created
task.created
task.updated
task.assigned
task.completed
task.deleted
comment.created
```

Audit model:

``` text
id
organization_id
actor_id
action
entity_type
entity_id
old_value
new_value
metadata
created_at
```

`old_value`, `new_value`, and `metadata` may use PostgreSQL JSONB.

Audit records should be immutable through the normal application API.

Endpoint:

``` http
GET /api/v1/organizations/:organizationID/audit-logs
```

------------------------------------------------------------------------

# 20. Transactional Outbox

The system must use the transactional outbox pattern for reliable
asynchronous events.

Incorrect approach:

``` text
BEGIN DB transaction
    |
    +-- Update task
    |
COMMIT
    |
Publish event
```

The publish operation can fail after the database transaction succeeds.

Instead:

``` text
BEGIN TRANSACTION
    |
    +-- Update task
    |
    +-- Insert outbox event
    |
COMMIT
```

Then:

``` text
Outbox Publisher
       |
       v
      SQS
```

Outbox fields:

``` text
id
event_type
aggregate_type
aggregate_id
organization_id
payload
created_at
published_at
attempts
last_error
```

------------------------------------------------------------------------

# 21. Domain Events

Initial events:

``` text
organization.created
member.added
member.role_changed
project.created
task.created
task.updated
task.assigned
task.completed
task.deleted
comment.created
```

Example event:

``` json
{
  "event_id": "uuid",
  "event_type": "task.completed",
  "aggregate_type": "task",
  "aggregate_id": "uuid",
  "organization_id": "uuid",
  "occurred_at": "timestamp",
  "payload": {}
}
```

Events should be designed for at-least-once delivery.

Consumers must therefore be idempotent.

------------------------------------------------------------------------

# 22. Background Workers

Create a separate worker executable:

``` text
cmd/worker/
```

Initial workers:

``` text
Outbox Publisher
Notification Worker
Reminder Worker
Cleanup Worker
```

The API should not perform slow asynchronous work synchronously.

Example:

``` text
POST /tasks
      |
      +-- Database transaction
      |
      +-- Outbox event
      |
      v
    201 Created

Later:

Outbox
   |
   v
 SQS
   |
   v
Worker
```

------------------------------------------------------------------------

# 23. AWS SQS

SQS should be used for asynchronous processing.

Requirements:

-   Visibility timeout
-   Retry handling
-   Dead-letter queue
-   Maximum receive count
-   Exponential backoff
-   Idempotent consumers
-   Graceful worker shutdown

Example:

``` text
Task Completed
      |
      v
Outbox
      |
      v
SQS
      |
      +----------+
      |          |
      v          v
Notification   Analytics
 Worker         Worker
```

For Phase 1, the system does not need multiple independent
microservices.

Workers can remain separate processes from the main API.

------------------------------------------------------------------------

# 24. Idempotency

Important write operations should support:

``` http
Idempotency-Key: <unique-key>
```

Example:

``` http
POST /api/v1/projects/:projectID/tasks
Idempotency-Key: abc-123
```

Store:

``` text
idempotency_keys
----------------
key
user_id
request_hash
status_code
response_body
created_at
expires_at
```

If a client retries the same request with the same key, the API should
return the original result instead of creating a duplicate resource.

The implementation must reject reuse of an idempotency key with a
different request payload.

------------------------------------------------------------------------

# 25. Redis

Redis should only be used where it solves a real problem.

Phase 1 use cases:

1.  Distributed rate limiting.
2.  Short-lived caching where useful.
3.  Optional temporary session/cache data.

Example:

``` text
organization:{organizationID}
```

Cache invalidation must happen after writes that affect cached data.

Redis should not become the source of truth for core domain data.

------------------------------------------------------------------------

# 26. Rate Limiting

Initial configurable defaults:

``` text
Unauthenticated:
10 requests/minute/IP

Authenticated:
100 requests/minute/user

Authentication:
5 requests/minute/IP
```

Rate limiting should be distributed through Redis so that multiple API
instances share the same limits.

The rate-limit policy should be configurable.

------------------------------------------------------------------------

# 27. Centralized Error Handling

All API errors should use a consistent format.

Example:

``` json
{
  "error": {
    "code": "TASK_NOT_FOUND",
    "message": "Task not found",
    "request_id": "req_123"
  }
}
```

Do not return:

-   SQL errors
-   stack traces
-   internal implementation details
-   JWT parsing internals
-   database credentials
-   sensitive information

Internal diagnostic information should remain internal.

------------------------------------------------------------------------

# 28. API Versioning

All public API routes should use:

``` text
/api/v1
```

Examples:

``` text
/api/v1/auth
/api/v1/organizations
/api/v1/projects
/api/v1/tasks
```

The architecture should allow future `/api/v2` without requiring a
complete rewrite.

------------------------------------------------------------------------

# 29. Context Propagation

Request context must flow through:

``` text
HTTP Handler
      |
      v
Service
      |
      v
Repository
      |
      v
PostgreSQL
```

Repository methods should accept:

``` go
context.Context
```

Do not create unrelated request contexts using `context.Background()`
inside repository methods.

Database operations should be cancelled when the originating HTTP
request is cancelled or times out.

------------------------------------------------------------------------

# 30. Database Design

PostgreSQL is the primary source of truth.

Core tables:

``` text
users
organizations
organization_members
projects
tasks
comments
labels
task_labels
refresh_tokens
audit_logs
outbox_events
idempotency_keys
```

Use:

-   Foreign keys
-   Unique constraints
-   Check constraints
-   Transactions
-   Appropriate indexes
-   Connection pooling
-   Query timeouts
-   Versioning/migrations

------------------------------------------------------------------------

# 31. Important Database Indexes

Indexes should be based on actual query patterns.

Examples:

``` sql
CREATE INDEX idx_tasks_project_created
ON tasks(project_id, created_at DESC);

CREATE INDEX idx_tasks_assignee_status
ON tasks(assignee_id, status);

CREATE INDEX idx_tasks_due_date
ON tasks(due_date)
WHERE status != 'DONE';
```

Indexes should be evaluated using actual query plans such as
`EXPLAIN ANALYZE`.

Do not add indexes without understanding their read/write trade-offs.

------------------------------------------------------------------------

# 32. Transactions

Use database transactions for operations requiring atomicity.

Examples:

### Organization creation

``` text
Create organization
+
Create owner membership
```

### Task update

``` text
Update task
+
Insert outbox event
+
Insert audit event
```

### Member role update

``` text
Update role
+
Insert audit event
```

------------------------------------------------------------------------

# 33. Soft Deletion

Core resources should support soft deletion where recovery/auditability
is useful.

Example:

``` text
deleted_at
```

Normal queries should exclude deleted records.

Example:

``` sql
WHERE deleted_at IS NULL
```

Resources that support soft deletion should provide restore operations
where appropriate.

------------------------------------------------------------------------

# 34. Authentication Security

Requirements:

-   Secure password hashing
-   Password validation
-   Short-lived access tokens
-   Refresh-token rotation
-   Refresh-token revocation
-   Session management
-   Authentication rate limiting
-   Secure JWT configuration
-   No token logging
-   No password logging
-   No password hashes in responses

------------------------------------------------------------------------

# 35. Authorization Security

Every protected resource must verify:

``` text
Authenticated User
       |
       v
Organization Membership
       |
       v
Required Role
       |
       v
Resource Access
```

Never trust organization/resource IDs supplied by the client without
server-side authorization checks.

------------------------------------------------------------------------

# 36. Request Validation

Validate:

-   UUIDs
-   Required fields
-   String lengths
-   Enum values
-   Dates
-   Pagination limits
-   Sort fields
-   Sort direction
-   Search parameters

Pagination limits must have a server-side maximum.

For example:

``` text
Default limit: 20
Maximum limit: 100
```

------------------------------------------------------------------------

# 37. Graceful Shutdown

The API and workers must gracefully handle termination signals.

Expected sequence:

``` text
SIGTERM
   |
   v
Stop accepting new requests
   |
   v
Wait for active requests
   |
   v
Stop workers
   |
   v
Finish safe in-flight work
   |
   v
Close Redis
   |
   v
Close PostgreSQL
   |
   v
Exit
```

A configurable shutdown timeout should be used.

------------------------------------------------------------------------

# 38. Health Endpoints

## Liveness

``` http
GET /health/live
```

Indicates that the application process is alive.

## Readiness

``` http
GET /health/ready
```

Checks required dependencies such as PostgreSQL and Redis.

Example:

``` json
{
  "status": "ready",
  "checks": {
    "postgres": "ok",
    "redis": "ok"
  }
}
```

------------------------------------------------------------------------

# 39. Configuration

Configuration should be environment-based.

Example:

``` text
APP_ENV
APP_PORT
DATABASE_URL
REDIS_URL

JWT_SECRET
JWT_ACCESS_TTL
JWT_REFRESH_TTL

AWS_REGION
SQS_QUEUE_URL
SQS_DLQ_URL
```

Requirements:

-   Validate configuration on startup.
-   Fail fast for missing required production configuration.
-   Never commit secrets.
-   Provide `.env.example`.

------------------------------------------------------------------------

# 40. API Documentation

Keep API documentation in the repository.

The initial project should use:

``` text
openapi.yaml
```

It should document:

-   Authentication
-   Endpoints
-   Request bodies
-   Query parameters
-   Responses
-   Error codes
-   Pagination
-   Authorization requirements
-   Idempotency
-   Rate limiting behavior

------------------------------------------------------------------------

# 41. Phase 1 Definition of Done

A feature is complete when:

-   API contract is defined.
-   Authorization is implemented.
-   Validation exists.
-   Database migration exists.
-   Relevant indexes exist.
-   Business logic is in the service layer.
-   Repository access is separated from handlers.
-   Errors use the standard API contract.
-   Request context is propagated.
-   Failure behavior is defined.
-   Documentation is updated.

Testing, CI/CD, observability, and containerized deployment are **not
Phase 1 completion requirements**. They belong to Phase 2.

------------------------------------------------------------------------

# 42. Phase 1 Implementation Roadmap

## Phase 1A --- Architecture Foundation

-   Refactor handlers.
-   Introduce service/use-case layer.
-   Introduce repository interfaces.
-   Propagate context.
-   Centralize errors.
-   Add validation.
-   Add API versioning.
-   Add graceful shutdown.
-   Add health/readiness endpoints.
-   Improve configuration handling.

## Phase 1B --- Authentication

-   Registration improvements.
-   Login improvements.
-   Access tokens.
-   Refresh tokens.
-   Token rotation.
-   Session storage.
-   Logout.
-   Logout all.
-   Authentication rate limiting.

## Phase 1C --- Multi-Tenancy

-   Organizations.
-   Organization members.
-   RBAC.
-   Membership management.
-   Organization-level authorization.

## Phase 1D --- Task Management

-   Projects.
-   Tasks.
-   Task statuses.
-   Priorities.
-   Assignees.
-   Comments.
-   Labels.
-   Soft deletion.
-   Restore.

## Phase 1E --- Database Engineering

-   Cursor pagination.
-   Filtering.
-   Sorting.
-   PostgreSQL search.
-   Index optimization.
-   Transactions.
-   Optimistic concurrency.

## Phase 1F --- Reliability

-   Audit logs.
-   Transactional outbox.
-   Domain events.
-   Idempotency.
-   Redis.
-   Distributed rate limiting.

## Phase 1G --- Async Processing

-   Worker process.
-   SQS.
-   Outbox publisher.
-   Notification worker.
-   Reminder worker.
-   Retry handling.
-   DLQ.
-   Idempotent consumers.
-   Graceful worker shutdown.

------------------------------------------------------------------------

# 43. Phase 2 --- Production Operations

After Phase 1 is stable, implement:

## Testing

-   Unit tests
-   Repository integration tests
-   PostgreSQL integration tests
-   API integration tests
-   E2E workflows
-   Worker tests
-   Failure/retry tests
-   Concurrency tests

## Observability

-   Structured logging
-   Prometheus metrics
-   Grafana dashboards
-   Loki
-   OpenTelemetry
-   Tempo
-   Distributed tracing
-   Alerts
-   Correlation IDs

## CI/CD

-   GitHub Actions
-   Formatting checks
-   Static analysis
-   Linting
-   Automated tests
-   Security scanning
-   Build pipeline
-   Release pipeline
-   Deployment pipeline

## Containerization and Deployment

-   Dockerfile
-   Multi-stage builds
-   Minimal runtime image
-   Docker Compose
-   Production container configuration
-   Container security
-   ECS/Kubernetes deployment as a later option
-   Health-based deployment
-   Graceful rolling updates

## Performance

-   k6/load testing
-   P50/P95/P99 measurements
-   Database performance testing
-   Connection pool tuning
-   Redis performance evaluation
-   Queue throughput testing

------------------------------------------------------------------------

# 44. Future Enhancements

Potential future features after Phase 2:

-   WebSocket real-time task updates
-   S3 attachments
-   Presigned upload URLs
-   Email notification preferences
-   Scheduled tasks
-   Recurring tasks
-   Webhooks
-   API keys
-   OIDC/SSO
-   Advanced analytics
-   Organization quotas
-   Public API integrations
-   Dedicated search service
-   Microservice extraction for selected workloads

These should only be introduced when there is a clear engineering
reason.

------------------------------------------------------------------------

# 45. Senior Backend Engineering Goals

The project should demonstrate practical knowledge of:

## Go

-   Context propagation
-   Interfaces
-   Dependency injection
-   Concurrency
-   Worker patterns
-   Graceful shutdown
-   Error handling

## PostgreSQL

-   Relational modeling
-   Transactions
-   Constraints
-   Indexes
-   Query optimization
-   Connection pooling
-   Optimistic locking
-   JSONB

## Distributed Systems

-   At-least-once delivery
-   Idempotent consumers
-   Transactional outbox
-   Retries
-   Dead-letter queues
-   Eventual consistency
-   Failure recovery

## Security

-   JWT
-   Refresh-token rotation
-   RBAC
-   Rate limiting
-   Input validation
-   Secure secrets

## API Design

-   REST
-   Versioning
-   Pagination
-   Filtering
-   Error contracts
-   Idempotency

------------------------------------------------------------------------

# 46. Final Architecture Goal

``` text
                         +----------------+
                         |    Clients     |
                         +-------+--------+
                                 |
                                 v
                         +---------------+
                         |   Go / Gin    |
                         |      API      |
                         +-------+-------+
                                 |
        +------------------------+------------------------+
        |                        |                        |
        v                        v                        v
      Auth                 Task Service            Project Service
        |                        |                        |
        +------------------------+------------------------+
                                 |
                                 v
                         +---------------+
                         |  PostgreSQL   |
                         |               |
                         | Domain Data   |
                         | Audit Logs    |
                         | Outbox        |
                         +-------+-------+
                                 |
                                 v
                         Outbox Publisher
                                 |
                                 v
                                SQS
                                 |
                  +--------------+--------------+
                  |              |              |
                  v              v              v
            Notification      Reminder       Other
              Worker          Worker         Workers

                         +---------------+
                         |     Redis     |
                         |               |
                         | Rate Limiting |
                         | Cache         |
                         +---------------+
```

------------------------------------------------------------------------

# 47. Engineering Principle

The goal is not to make the Todo API as large as possible.

The goal is to demonstrate how a senior backend engineer designs a
system for:

``` text
Correctness
    +
Security
    +
Maintainability
    +
Reliability
    +
Scalability
```

Every technology should have a reason for existing.

Prefer:

``` text
Simple architecture
+
Strong engineering decisions
+
Clear trade-offs
```

over:

``` text
Many technologies
+
Many services
+
Little justification
```

Phase 1 should therefore remain a modular monolith with PostgreSQL,
Redis, SQS, and workers.

Phase 2 will add the operational engineering required to run the system
in production.
