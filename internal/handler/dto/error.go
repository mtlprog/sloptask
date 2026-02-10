package dto

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mtlprog/sloptask/internal/domain"
)

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error code and message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewErrorResponse creates a new error response.
func NewErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
}

// MapDomainError maps domain errors to HTTP status codes and error codes.
func MapDomainError(err error) (status int, code string, message string) {
	message = err.Error()

	switch {
	// Task errors
	case errors.Is(err, domain.ErrTaskNotFound):
		return http.StatusNotFound, "TASK_NOT_FOUND", message
	case errors.Is(err, domain.ErrTaskAlreadyClaimed):
		return http.StatusConflict, "TASK_ALREADY_CLAIMED", message
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusConflict, "INVALID_TRANSITION", message
	case errors.Is(err, domain.ErrUnresolvedBlockers):
		return http.StatusConflict, "UNRESOLVED_BLOCKERS", message
	case errors.Is(err, domain.ErrCyclicDependency):
		return http.StatusConflict, "CYCLIC_DEPENDENCY", message

	// Permission errors
	case errors.Is(err, domain.ErrPermissionDenied):
		return http.StatusForbidden, "INSUFFICIENT_ACCESS", message
	case errors.Is(err, domain.ErrNotTaskOwner):
		return http.StatusForbidden, "INSUFFICIENT_ACCESS", message
	case errors.Is(err, domain.ErrNotTaskCreator):
		return http.StatusForbidden, "INSUFFICIENT_ACCESS", message

	// Agent errors
	case errors.Is(err, domain.ErrAgentNotFound):
		return http.StatusUnauthorized, "INVALID_TOKEN", message
	case errors.Is(err, domain.ErrAgentInactive):
		return http.StatusUnauthorized, "AGENT_INACTIVE", message
	case errors.Is(err, domain.ErrInvalidToken):
		return http.StatusUnauthorized, "INVALID_TOKEN", message

	// Workspace errors
	case errors.Is(err, domain.ErrWorkspaceNotFound):
		return http.StatusNotFound, "WORKSPACE_NOT_FOUND", message

	// Validation errors
	case errors.Is(err, domain.ErrInvalidStatus):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR", message
	case errors.Is(err, domain.ErrInvalidVisibility):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR", message
	case errors.Is(err, domain.ErrInvalidPriority):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR", message
	case errors.Is(err, domain.ErrEmptyComment):
		return http.StatusUnprocessableEntity, "VALIDATION_ERROR", message

	// Default: internal server error
	default:
		// CRITICAL: Log unmapped error for debugging
		slog.Error("unmapped domain error returned to client",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
		)
		return http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
	}
}
