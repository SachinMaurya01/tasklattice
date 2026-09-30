package handlers

import (
	"net/http"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// ProjectHandler exposes the project endpoints.
type ProjectHandler struct {
	projects *services.ProjectService
}

// NewProjectHandler wires the project endpoints.
func NewProjectHandler(projects *services.ProjectService) *ProjectHandler {
	return &ProjectHandler{projects: projects}
}

type projectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type projectPatchRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// Create handles POST /api/v1/organizations/:organizationID/projects.
func (h *ProjectHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	p, err := h.projects.Create(c.Request.Context(), userID, c.Param("organizationID"), req.Name, req.Description)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, p)
}

// List handles GET /api/v1/organizations/:organizationID/projects.
func (h *ProjectHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	projects, err := h.projects.List(c.Request.Context(), userID, c.Param("organizationID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, projects)
}

// Get handles GET /api/v1/projects/:projectID.
func (h *ProjectHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	p, err := h.projects.Get(c.Request.Context(), userID, c.Param("projectID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// Update handles PATCH /api/v1/projects/:projectID.
func (h *ProjectHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req projectPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	if req.Name == nil && req.Description == nil {
		apperrors.Respond(c, apperrors.BadRequest("At least one field (name or description) must be provided"))
		return
	}
	ctx := c.Request.Context()
	projectID := c.Param("projectID")
	current, err := h.projects.Get(ctx, userID, projectID)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	name, desc := current.Name, current.Description
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		desc = *req.Description
	}
	p, err := h.projects.Update(ctx, userID, projectID, name, desc)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// Delete handles DELETE /api/v1/projects/:projectID.
func (h *ProjectHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.projects.Delete(c.Request.Context(), userID, c.Param("projectID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Project deleted"})
}
