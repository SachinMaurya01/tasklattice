package handlers

import (
	"net/http"
	"strconv"
	"strings"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/services"

	"github.com/gin-gonic/gin"
)

// AuditHandler exposes the audit-log endpoint.
type AuditHandler struct {
	audits *services.AuditService
}

// NewAuditHandler wires the audit endpoints.
func NewAuditHandler(audits *services.AuditService) *AuditHandler {
	return &AuditHandler{audits: audits}
}

// List handles GET /api/v1/organizations/:organizationID/audit-logs.
func (h *AuditHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	limit, offset := 0, 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			apperrors.Respond(c, apperrors.BadRequest("Invalid limit"))
			return
		}
		limit = n
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			apperrors.Respond(c, apperrors.BadRequest("Invalid offset"))
			return
		}
		offset = n
	}
	logs, err := h.audits.List(c.Request.Context(), userID, c.Param("organizationID"), limit, offset)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, logs)
}
