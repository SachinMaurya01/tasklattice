package repository

import (
	"context"
	"fmt"

	"tasklattice/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateTodo inserts a legacy todo for a user.
func CreateTodo(ctx context.Context, pool *pgxpool.Pool, title string, completed bool, userID string) (*models.Todo, error) {
	const query = `
			INSERT INTO todos (title, completed, user_id)
			VALUES ($1, $2, $3)
			RETURNING id, title, completed, created_at, updated_at, user_id
	`

	var todo models.Todo

	err := pool.QueryRow(ctx, query, title, completed, userID).Scan(
		&todo.ID,
		&todo.Title,
		&todo.Completed,
		&todo.CreatedAt,
		&todo.UpdatedAt,
		&todo.UserID,
	)
	if err != nil {
		return nil, err
	}

	return &todo, nil
}

// GetAllTodos lists a user's legacy todos, newest first.
func GetAllTodos(ctx context.Context, pool *pgxpool.Pool, userID string) ([]models.Todo, error) {
	const query = `
		SELECT id, title, completed, created_at, updated_at, user_id
		FROM todos
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	todos := []models.Todo{}

	for rows.Next() {
		var todo models.Todo

		err = rows.Scan(
			&todo.ID,
			&todo.Title,
			&todo.Completed,
			&todo.CreatedAt,
			&todo.UpdatedAt,
			&todo.UserID,
		)
		if err != nil {
			return nil, err
		}

		todos = append(todos, todo)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return todos, nil
}

// GetToDoByID fetches one legacy todo scoped to a user.
func GetToDoByID(ctx context.Context, pool *pgxpool.Pool, id int, userID string) (*models.Todo, error) {
	const query = `
		SELECT id, title, completed, created_at, updated_at, user_id
		FROM todos
		WHERE id = $1 AND user_id = $2
	`

	var todo models.Todo

	err := pool.QueryRow(ctx, query, id, userID).Scan(
		&todo.ID,
		&todo.Title,
		&todo.Completed,
		&todo.CreatedAt,
		&todo.UpdatedAt,
		&todo.UserID,
	)
	if err != nil {
		return nil, err
	}

	return &todo, nil
}

// UpdateToDo updates a legacy todo scoped to a user.
func UpdateToDo(ctx context.Context, pool *pgxpool.Pool, id int, title string, completed bool, userID string) (*models.Todo, error) {
	const query = `
		UPDATE todos
		SET title = $1, completed = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND user_id = $4
		RETURNING id, title, completed, created_at, updated_at, user_id
	`

	var todo models.Todo

	err := pool.QueryRow(ctx, query, title, completed, id, userID).Scan(
		&todo.ID,
		&todo.Title,
		&todo.Completed,
		&todo.CreatedAt,
		&todo.UpdatedAt,
		&todo.UserID,
	)
	if err != nil {
		return nil, err
	}

	return &todo, nil
}

// DeleteToDo hard-deletes a legacy todo scoped to a user.
func DeleteToDo(ctx context.Context, pool *pgxpool.Pool, id int, userID string) error {
	const query = `
		DELETE FROM todos
		WHERE id = $1 AND user_id = $2
	`

	commandTag, err := pool.Exec(ctx, query, id, userID)
	if err != nil {
		return err
	}

	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("todo with id %d not found", id)
	}

	return nil
}
