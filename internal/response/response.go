package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse represents the standardized JSON error contract for HookOps.
type ErrorResponse struct {
	Error string `json:"error"`
}

// PaginationMeta contains pagination details for collection endpoints.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PaginatedResponse wraps a slice of items with pagination metadata.
type PaginatedResponse struct {
	Data any            `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// --- Success Responses ---

// OK sends a 200 OK response with the provided payload.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Created sends a 201 Created response with the provided payload.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// Accepted sends a 202 Accepted response with the provided payload.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, data)
}

// NoContent sends a 204 No Content response.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

// Paginated sends a 200 OK response with an items slice and pagination metadata.
func Paginated(c *gin.Context, data any, meta PaginationMeta) {
	c.JSON(http.StatusOK, PaginatedResponse{
		Data: data,
		Meta: meta,
	})
}

// --- Error Responses ---

// Error sends a custom HTTP status code with a standardized ErrorResponse.
func Error(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, ErrorResponse{
		Error: message,
	})
}

// BadRequest sends a 400 Bad Request error.
func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
}

// Unauthorized sends a 401 Unauthorized error.
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, message)
}

// Forbidden sends a 403 Forbidden error.
func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, message)
}

// NotFound sends a 404 Not Found error.
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, message)
}

// Conflict sends a 409 Conflict error.
func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, message)
}

// InternalServerError sends a 500 Internal Server Error.
// We purposely use a generic message to prevent leaking internal database/system errors to clients.
func InternalServerError(c *gin.Context) {
	Error(c, http.StatusInternalServerError, "internal server error")
}
