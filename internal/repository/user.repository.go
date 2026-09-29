package repository

import (
	"context"

	"Todo-App/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository abstracts user persistence. Services depend on this
// interface; handlers never touch the pool directly.
type UserRepository interface {
	CreateUser(ctx context.Context, user *models.User) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, id string) (*models.User, error)
}

// PostgresUserRepository is the PostgreSQL UserRepository implementation.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository wires a UserRepository to a pool.
func NewUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

// CreateUser inserts a user. The caller passes a request-scoped ctx;
// uniqueness of email is enforced by the database constraint.
func (r *PostgresUserRepository) CreateUser(ctx context.Context, user *models.User) (*models.User, error) {
	const query = `
		INSERT INTO users (email, password)
		VALUES ($1, $2)
		RETURNING id, email, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query, user.Email, user.Password).Scan(
		&user.ID,
		&user.Email,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByEmail fetches a user including the password hash (for login only;
// never serialize it into responses).
func (r *PostgresUserRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = `
		SELECT id, email, password, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	var user models.User

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID fetches a user by id.
func (r *PostgresUserRepository) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	const query = `
		SELECT id, email, password, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var user models.User

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
