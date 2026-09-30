package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// TaskHandler exposes the task endpoints.
type TaskHandler struct {
	tasks *services.TaskService
}

// NewTaskHandler wires the task endpoints.
func NewTaskHandler(tasks *services.TaskService) *TaskHandler {
	return &TaskHandler{tasks: tasks}
}

type taskCreateRequest struct {
	Title       string  `json:"title" binding:"required"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
	AssigneeID  *string `json:"assignee_id"`
	DueDate     *string `json:"due_date"`
}

type taskPatchRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Status        *string `json:"status"`
	Priority      *string `json:"priority"`
	AssigneeID    *string `json:"assignee_id"`
	ClearAssignee bool    `json:"clear_assignee"`
	DueDate       *string `json:"due_date"`
	ClearDueDate  bool    `json:"clear_due_date"`
	Version       int     `json:"version" binding:"required"`
}

// parseOptionalDate parses an RFC3339 date, treating "" as unset.
func parseOptionalDate(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// normalizeAssignee treats "" as unset.
func normalizeAssignee(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// Create handles POST /api/v1/projects/:projectID/tasks.
func (h *TaskHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req taskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	due, err := parseOptionalDate(req.DueDate)
	if err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid due_date, use RFC3339"))
		return
	}
	t, err := h.tasks.Create(c.Request.Context(), userID, c.Param("projectID"), services.TaskCreate{
		Title:       req.Title,
		Description: req.Description,
		Status:      strings.ToUpper(strings.TrimSpace(req.Status)),
		Priority:    strings.ToUpper(strings.TrimSpace(req.Priority)),
		AssigneeID:  normalizeAssignee(req.AssigneeID),
		DueDate:     due,
	})
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

// taskQueryFromParams validates list query params into a TaskQuery.
func taskQueryFromParams(c *gin.Context) (models.TaskQuery, error) {
	var q models.TaskQuery
	q.Status = strings.ToUpper(strings.TrimSpace(c.Query("status")))
	q.Priority = strings.ToUpper(strings.TrimSpace(c.Query("priority")))
	q.AssigneeID = strings.TrimSpace(c.Query("assignee_id"))
	q.CreatorID = strings.TrimSpace(c.Query("creator_id"))
	q.LabelID = strings.TrimSpace(c.Query("label"))
	q.Search = strings.TrimSpace(c.Query("search"))
	q.Sort = strings.TrimSpace(c.Query("sort"))
	q.Order = strings.ToLower(strings.TrimSpace(c.Query("order")))
	q.Cursor = c.Query("cursor")

	parseDate := func(key string) (*time.Time, error) {
		raw := strings.TrimSpace(c.Query(key))
		if raw == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, err
		}
		return &t, nil
	}
	var err error
	if q.DueBefore, err = parseDate("due_before"); err != nil {
		return q, apperrors.BadRequest("Invalid due_before, use RFC3339")
	}
	if q.DueAfter, err = parseDate("due_after"); err != nil {
		return q, apperrors.BadRequest("Invalid due_after, use RFC3339")
	}
	if q.CreatedBefore, err = parseDate("created_before"); err != nil {
		return q, apperrors.BadRequest("Invalid created_before, use RFC3339")
	}
	if q.CreatedAfter, err = parseDate("created_after"); err != nil {
		return q, apperrors.BadRequest("Invalid created_after, use RFC3339")
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, apperrors.BadRequest("Invalid limit")
		}
		q.Limit = n
	}
	return q, nil
}

// List handles GET /api/v1/projects/:projectID/tasks.
func (h *TaskHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	q, err := taskQueryFromParams(c)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	page, err := h.tasks.List(c.Request.Context(), userID, c.Param("projectID"), q)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// Get handles GET /api/v1/tasks/:taskID.
func (h *TaskHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	t, err := h.tasks.Get(c.Request.Context(), userID, c.Param("taskID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// Update handles PATCH /api/v1/tasks/:taskID.
func (h *TaskHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req taskPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	if req.Title == nil && req.Description == nil && req.Status == nil &&
		req.Priority == nil && req.AssigneeID == nil && !req.ClearAssignee &&
		req.DueDate == nil && !req.ClearDueDate {
		apperrors.Respond(c, apperrors.BadRequest("At least one field must be provided"))
		return
	}
	due, err := parseOptionalDate(req.DueDate)
	if err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid due_date, use RFC3339"))
		return
	}
	patch := services.TaskPatch{
		Title:       req.Title,
		Description: req.Description,
		ClearAssign: req.ClearAssignee,
		DueDate:     due,
		ClearDue:    req.ClearDueDate,
		Version:     req.Version,
	}
	if req.Status != nil {
		upper := strings.ToUpper(strings.TrimSpace(*req.Status))
		patch.Status = &upper
	}
	if req.Priority != nil {
		upper := strings.ToUpper(strings.TrimSpace(*req.Priority))
		patch.Priority = &upper
	}
	if req.AssigneeID != nil {
		patch.AssigneeID = normalizeAssignee(req.AssigneeID)
	}
	t, err := h.tasks.Update(c.Request.Context(), userID, c.Param("taskID"), patch)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// Delete handles DELETE /api/v1/tasks/:taskID.
func (h *TaskHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.tasks.Delete(c.Request.Context(), userID, c.Param("taskID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Task deleted"})
}

// Restore handles POST /api/v1/tasks/:taskID/restore.
func (h *TaskHandler) Restore(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	t, err := h.tasks.Restore(c.Request.Context(), userID, c.Param("taskID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}
