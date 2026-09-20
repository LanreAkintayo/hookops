package response_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/response"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestResponseHelpers(t *testing.T) {
	t.Run("OK sends 200 with JSON payload", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.OK(c, gin.H{"message": "success"})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"message":"success"}`, rec.Body.String())
	})

	t.Run("Created sends 201 with JSON payload", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.Created(c, gin.H{"id": "123"})

		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.JSONEq(t, `{"id":"123"}`, rec.Body.String())
	})

	t.Run("NoContent sends 204", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.NoContent(c)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("BadRequest sends 400 with standardized error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.BadRequest(c, "invalid input")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.JSONEq(t, `{"error":"invalid input"}`, rec.Body.String())
	})

	t.Run("Unauthorized sends 401 with standardized error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.Unauthorized(c, "invalid token")

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.JSONEq(t, `{"error":"invalid token"}`, rec.Body.String())
	})

	t.Run("Forbidden sends 403 with standardized error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.Forbidden(c, "access denied")

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.JSONEq(t, `{"error":"access denied"}`, rec.Body.String())
	})

	t.Run("NotFound sends 404 with standardized error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.NotFound(c, "application not found")

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.JSONEq(t, `{"error":"application not found"}`, rec.Body.String())
	})

	t.Run("Conflict sends 409 with standardized error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.Conflict(c, "duplicate key")

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.JSONEq(t, `{"error":"duplicate key"}`, rec.Body.String())
	})

	t.Run("InternalServerError sends 500 with generic masked error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		response.InternalServerError(c)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.JSONEq(t, `{"error":"internal server error"}`, rec.Body.String())
	})

	t.Run("Paginated sends 200 with data and meta", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		items := []string{"app_1", "app_2"}
		meta := response.PaginationMeta{
			Page:       1,
			PerPage:    10,
			Total:      2,
			TotalPages: 1,
		}

		response.Paginated(c, items, meta)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"data":["app_1","app_2"],"meta":{"page":1,"per_page":10,"total":2,"total_pages":1}}`, rec.Body.String())
	})
}
