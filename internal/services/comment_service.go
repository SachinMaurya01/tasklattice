package services

import (
	"context"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"
)

// CommentService implements task comments with author permissions.
type CommentService struct {
	comments repository.CommentRepository
	tasks    repository.TaskRepository
	orgs     repository.OrganizationRepository
}

// NewCommentService wires the comment use cases.
func NewCommentService(comments repository.CommentRepository, tasks repository.TaskRepository, orgs repository.OrganizationRepository) *CommentService {
	return &CommentService{comments: comments, tasks: tasks, orgs: orgs}
}

// resolveTask loads a non-deleted task and enforces min role in its org.
func (s *CommentService) resolveTask(ctx context.Context, userID, taskID string, min models.Role) (*models.Task, error) {
	if !models.IsUUID(taskID) {
		return nil, apperrors.BadRequest("Invalid task ID")
	}
	t, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Task not found")
		}
		return nil, apperrors.Internal()
	}
	if _, err := requireRole(ctx, s.orgs, t.OrganizationID, userID, min); err != nil {
		return nil, err
	}
	return t, nil
}

// Create adds a comment to a task. Member only.
func (s *CommentService) Create(ctx context.Context, userID, taskID, body string) (*models.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 10000 {
		return nil, apperrors.BadRequest("Comment body must be 1-10000 characters")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleMember)
	if err != nil {
		return nil, err
	}
	c := &models.Comment{OrganizationID: t.OrganizationID, TaskID: t.ID, AuthorID: &userID, Body: body}
	cs := models.ChangeSet{
		Events: []*models.OutboxEvent{
			models.MustOutboxEvent(
				models.EventCommentCreated, models.EntityComment, "", t.OrganizationID,
				map[string]any{"task_id": t.ID},
			),
		},
		Audit: models.NewAuditLog(t.OrganizationID, userID, models.AuditCommentCreated, models.EntityComment, "",
			nil, map[string]any{"task_id": t.ID}, nil),
	}
	if err := s.comments.Create(ctx, c, cs); err != nil {
		return nil, apperrors.Internal()
	}
	return c, nil
}

// List returns non-deleted comments of a task for a member.
func (s *CommentService) List(ctx context.Context, userID, taskID string) ([]models.Comment, error) {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	t, err := s.resolveTask(ctx, userID, taskID, models.RoleViewer)
	if err != nil {
		return nil, err
	}
	comments, err := s.comments.ListByTask(ctx, t.ID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	return comments, nil
}

// resolveComment loads a comment and its task org membership.
func (s *CommentService) resolveComment(ctx context.Context, userID, commentID string) (*models.Comment, *models.OrganizationMember, error) {
	if !models.IsUUID(commentID) {
		return nil, nil, apperrors.BadRequest("Invalid comment ID")
	}
	c, err := s.comments.GetByID(ctx, commentID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil, apperrors.NotFound("Comment not found")
		}
		return nil, nil, apperrors.Internal()
	}
	if _, err := s.tasks.GetByID(ctx, c.TaskID); err != nil {
		if isNoRows(err) {
			return nil, nil, apperrors.NotFound("Comment not found")
		}
		return nil, nil, apperrors.Internal()
	}
	m, err := requireRole(ctx, s.orgs, c.OrganizationID, userID, models.RoleViewer)
	if err != nil {
		return nil, nil, err
	}
	return c, m, nil
}

// Update edits a comment. Author or admin only.
func (s *CommentService) Update(ctx context.Context, userID, commentID, body string) (*models.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 10000 {
		return nil, apperrors.BadRequest("Comment body must be 1-10000 characters")
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	c, m, err := s.resolveComment(ctx, userID, commentID)
	if err != nil {
		return nil, err
	}
	if (c.AuthorID == nil || *c.AuthorID != userID) && !m.Role.AtLeast(models.RoleAdmin) {
		return nil, apperrors.Forbidden("Only the author or an admin may edit this comment")
	}
	out, err := s.comments.UpdateBody(ctx, c.ID, body)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.NotFound("Comment not found")
		}
		return nil, apperrors.Internal()
	}
	return out, nil
}

// Delete soft-deletes a comment. Author or admin only.
func (s *CommentService) Delete(ctx context.Context, userID, commentID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	c, m, err := s.resolveComment(ctx, userID, commentID)
	if err != nil {
		return err
	}
	if (c.AuthorID == nil || *c.AuthorID != userID) && !m.Role.AtLeast(models.RoleAdmin) {
		return apperrors.Forbidden("Only the author or an admin may delete this comment")
	}
	if err := s.comments.SoftDelete(ctx, c.ID); err != nil {
		if isNoRows(err) {
			return apperrors.NotFound("Comment not found")
		}
		return apperrors.Internal()
	}
	return nil
}
