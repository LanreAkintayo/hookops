package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/LanreAkintayo/hookops/internal/response"
)

// Recovery returns a Gin middleware that intercepts unexpected panics,
// logs the stack trace to Zerolog, and returns a sanitized 500 JSON response.
func Recovery(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())

				log.Error().
					Interface("panic", r).
					Str("stack", stack).
					Str("path", c.Request.URL.Path).
					Str("method", c.Request.Method).
					Msg("panic recovered in HTTP handler")

				response.InternalServerError(c)
				c.Abort()
			}
		}()

		c.Next()
	}
}
