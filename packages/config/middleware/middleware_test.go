package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestIDGeneratesIDWhenMissing(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		rid := GetRequestID(c)
		if rid == "" {
			t.Errorf("expected request_id to be set")
		}
		if len(rid) != 24 {
			t.Errorf("request_id length = %d, want 24 hex chars", len(rid))
		}
		c.String(http.StatusOK, rid)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	headerRID := w.Header().Get(RequestIDHeader)
	if headerRID == "" {
		t.Errorf("X-Request-ID header missing")
	}
}

func TestRequestIDPreservesProvidedID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		rid := GetRequestID(c)
		if rid != "test-req-id-123" {
			t.Errorf("request_id = %q, want 'test-req-id-123'", rid)
		}
		c.String(http.StatusOK, rid)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ping", nil)
	req.Header.Set(RequestIDHeader, "test-req-id-123")
	r.ServeHTTP(w, req)

	if w.Header().Get(RequestIDHeader) != "test-req-id-123" {
		t.Errorf("X-Request-ID header = %q, want 'test-req-id-123'", w.Header().Get(RequestIDHeader))
	}
}

func TestRequestIDInContext(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/ping", func(c *gin.Context) {
		val := c.Request.Context().Value(RequestIDKey)
		if val == nil {
			t.Errorf("request_id not found in context")
		}
		if s, ok := val.(string); !ok || s == "" {
			t.Errorf("request_id context value invalid: %v", val)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRecoveryHandlesPanic(t *testing.T) {
	// Use discard logger for test
	r := gin.New()
	r.Use(RequestID())
	r.Use(Recovery(nil))
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCORSDefault(t *testing.T) {
	r := gin.New()
	r.Use(CORS(DefaultCORSConfig()))
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/ok", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("CORS allow-origin = %q, want *", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(50 * time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(200 * time.Millisecond)
		c.String(http.StatusOK, "done")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/slow", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("timeout status = %d, want 504", w.Code)
	}
}

func TestTimeoutAllowsFastRequests(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(1 * time.Second))
	r.GET("/fast", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/fast", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("fast request status = %d, want 200", w.Code)
	}
}

func TestContextPassedToHandler(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.Use(Timeout(1 * time.Second))
	r.GET("/ctx", func(c *gin.Context) {
		if c.Request.Context() == context.Background() {
			t.Error("expected context to be request context")
		}
		if c.Request.Context().Err() != nil {
			t.Error("expected context not cancelled")
		}
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ctx", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
