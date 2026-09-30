package handlers

import (
	"net/http"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// LabelHandler exposes the label endpoints.
type LabelHandler struct {
	labels *services.LabelService
}

// NewLabelHandler wires the label endpoints.
func NewLabelHandler(labels *services.LabelService) *LabelHandler {
	return &LabelHandler{labels: labels}
}

type labelRequest struct {
	Name string `json:"name" binding:"required"`
}

type attachLabelRequest struct {
	LabelID string `json:"label_id" binding:"required"`
}

// Create handles POST /api/v1/projects/:projectID/labels.
func (h *LabelHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req labelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	l, err := h.labels.Create(c.Request.Context(), userID, c.Param("projectID"), req.Name)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, l)
}

// List handles GET /api/v1/projects/:projectID/labels.
func (h *LabelHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	labels, err := h.labels.List(c.Request.Context(), userID, c.Param("projectID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, labels)
}

// Rename handles PATCH /api/v1/labels/:labelID.
func (h *LabelHandler) Rename(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req labelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	l, err := h.labels.Rename(c.Request.Context(), userID, c.Param("labelID"), req.Name)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, l)
}

// Delete handles DELETE /api/v1/labels/:labelID.
func (h *LabelHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.labels.Delete(c.Request.Context(), userID, c.Param("labelID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Label deleted"})
}

// Attach handles POST /api/v1/tasks/:taskID/labels.
func (h *LabelHandler) Attach(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req attachLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	if err := h.labels.Attach(c.Request.Context(), userID, c.Param("taskID"), req.LabelID); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Label attached"})
}

// Detach handles DELETE /api/v1/tasks/:taskID/labels/:labelID.
func (h *LabelHandler) Detach(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.labels.Detach(c.Request.Context(), userID, c.Param("taskID"), c.Param("labelID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Label detached"})
}
