package handlers

import (
	"net/http"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// currentUserID extracts the authenticated user id or renders 401.
func currentUserID(c *gin.Context) (string, bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		apperrors.Respond(c, apperrors.Unauthorized("Authentication required"))
		return "", false
	}
	id, ok := raw.(string)
	if !ok || id == "" {
		apperrors.Respond(c, apperrors.Unauthorized("Authentication required"))
		return "", false
	}
	return id, true
}

// OrganizationHandler exposes the organization and membership endpoints.
type OrganizationHandler struct {
	orgs *services.OrganizationService
}

// NewOrganizationHandler wires the organization endpoints.
func NewOrganizationHandler(orgs *services.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{orgs: orgs}
}

type orgRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type orgPatchRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type memberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

type memberPatchRequest struct {
	Role string `json:"role" binding:"required"`
}

// Create handles POST /api/v1/organizations.
func (h *OrganizationHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req orgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	org, err := h.orgs.Create(c.Request.Context(), userID, req.Name, req.Description)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, org)
}

// List handles GET /api/v1/organizations.
func (h *OrganizationHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orgs, err := h.orgs.List(c.Request.Context(), userID)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, orgs)
}

// Get handles GET /api/v1/organizations/:organizationID.
func (h *OrganizationHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	org, err := h.orgs.Get(c.Request.Context(), userID, c.Param("organizationID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, org)
}

// Update handles PATCH /api/v1/organizations/:organizationID.
func (h *OrganizationHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req orgPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	if req.Name == nil && req.Description == nil {
		apperrors.Respond(c, apperrors.BadRequest("At least one field (name or description) must be provided"))
		return
	}
	ctx := c.Request.Context()
	orgID := c.Param("organizationID")
	current, err := h.orgs.Get(ctx, userID, orgID)
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
	org, err := h.orgs.Update(ctx, userID, orgID, name, desc)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, org)
}

// Delete handles DELETE /api/v1/organizations/:organizationID.
func (h *OrganizationHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.orgs.Delete(c.Request.Context(), userID, c.Param("organizationID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Organization deleted"})
}

// ListMembers handles GET /api/v1/organizations/:organizationID/members.
func (h *OrganizationHandler) ListMembers(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	members, err := h.orgs.ListMembers(c.Request.Context(), userID, c.Param("organizationID"))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// AddMember handles POST /api/v1/organizations/:organizationID/members.
func (h *OrganizationHandler) AddMember(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req memberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	m, err := h.orgs.AddMember(c.Request.Context(), userID, c.Param("organizationID"), req.UserID, roleFromString(req.Role))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// UpdateMember handles PATCH /api/v1/organizations/:organizationID/members/:userID.
func (h *OrganizationHandler) UpdateMember(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req memberPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	m, err := h.orgs.UpdateMemberRole(c.Request.Context(), userID, c.Param("organizationID"), c.Param("userID"), roleFromString(req.Role))
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// RemoveMember handles DELETE /api/v1/organizations/:organizationID/members/:userID.
func (h *OrganizationHandler) RemoveMember(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := h.orgs.RemoveMember(c.Request.Context(), userID, c.Param("organizationID"), c.Param("userID")); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member removed"})
}

// roleFromString normalizes a role name; unknown values fail Valid() downstream.
func roleFromString(s string) models.Role {
	return models.Role(strings.ToUpper(strings.TrimSpace(s)))
}
