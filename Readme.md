# TaskLattice

## Getting Started

### 1. Download/Clone the Repository

### 2. Install Dependencies

```bash
go mod download
```

### 3. Set Up PostgreSQL Database

Create a new database in PostgreSQL:

```sql
CREATE DATABASE tasklattice;
```

### 4. Configure Environment Variables

Create a `.env` file in the root directory:

```env
DATABASE_URL=postgres://username:password@localhost:5432/tasklattice?sslmode=disable
PORT=3000
JWT_SECRET=your-secure-jwt-secret-key
```

### 5. Run Database Migrations

Using the migrate CLI:

```bash
migrate -path migrations -database "your_database_url" up
```

Or using the PowerShell script:

```powershell
.\scripts\migrate.ps1 up
```

### 6. Start the Server

```bash
go run ./cmd/api
```

Or with Air for hot reloading:

```bash
air
```

The API will be available at `http://localhost:3000`

## Project Structure

```
tasklattice/
├── cmd/
│   └── api/
│       └── main.go              # Application entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Environment configuration
│   ├── database/
│   │   └── postgres.go          # Database connection
│   ├── handlers/
│   │   ├── todo_handler.go      # Todo route handlers
│   │   └── user_handler.go      # Auth route handlers
│   ├── middleware/
│   │   └── auth_middleware.go   # JWT authentication middleware
│   ├── models/
│   │   ├── todo.go              # Todo model
│   │   └── user.go              # User model
│   └── repository/
│       ├── todo_repository.go   # Todo database operations
│       └── user_repository.go   # User database operations
├── migrations/
│   ├── 000001_create_todos_api_table.up.sql
│   ├── 000001_create_todos_api_table.down.sql
│   ├── 000002_create_users_api_table.up.sql
│   ├── 000002_create_users_api_table.down.sql
│   ├── 000003_add_user_id_to_todos_table.up.sql
│   └── 000003_add_user_id_to_todos_table.down.sql
├── scripts/
│   └── migrate.ps1              # Migration helper script
├── .air.toml                    # Air configuration
├── .env                         # Environment variables (create this)
├── go.mod                       # Go module definition
└── go.sum                       # Go dependencies checksum
```


## Technologies Used

- **Go 1.26+**: Backend programming language
- **Gin**: HTTP web framework
- **PostgreSQL**: Relational database
- **pgx/v5**: PostgreSQL driver and connection pool
- **JWT**: JSON Web Tokens for authentication
- **bcrypt**: Password hashing
- **golang-migrate**: Database migrations
- **Air**: Hot reloading for development
- **godotenv**: Environment variable management