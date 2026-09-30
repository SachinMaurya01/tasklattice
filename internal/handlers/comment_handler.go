package handlers

import (
	"net/http"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// CommentHandler exposes the comment endpoints.
type CommentHandler struct {
	comments *services.CommentService
}

// NewCommentHandler wires the comment endpoints.
func NewCommentHandler(comments *services.CommentService) *CommentHandler {
	return &CommentHandler{comments: comments}
}

type commentRequest struct {
	Body string `json:"body" binding:"required"`
}

// Create handles POST /api/v1/tasks/:taskID/comments.
func (h *CommentHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req commentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	comment, err := h.comments.Create(c.Request.Context(), userID, c.Param("taskID"), req.Body)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, comment)
}

// List handles GET /api/v1/tasks/:taskID/comments.
func (h *CommentHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	comments, err := h.comments.List(c.Request.Context(), userID, c.Param("taskID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

// Update handles PATCH /api/v1/comments/:commentID.
func (h *CommentHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req commentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	comment, err := h.comments.Update(c.Request.Context(), userID, c.Param("commentID"), req.Body)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, comment)
}

// Delete handles DELETE /api/v1/comments/:commentID.
func (h *CommentHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.comments.Delete(c.Request.Context(), userID, c.Param("commentID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
}
