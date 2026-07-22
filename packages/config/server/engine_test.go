package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/airport-panel/config"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestHealthzEndpoint(t *testing.T) {
	opts := DefaultOptions("test-service", "0")
	opts.Logger = testLogger()
	s := New(opts)
	srv := httptest.NewServer(s.Engine())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want 'ok'", body["status"])
	}
	if body["service"] != "test-service" {
		t.Errorf("service field = %q, want 'test-service'", body["service"])
	}
}

func TestReadyzEndpoint(t *testing.T) {
	opts := DefaultOptions("test-service", "0")
	opts.Logger = testLogger()
	s := New(opts)
	srv := httptest.NewServer(s.Engine())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	opts := DefaultOptions("test-service", "0")
	opts.Logger = testLogger()
	s := New(opts)
	srv := httptest.NewServer(s.Engine())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	buf := make([]byte, 2048)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if len(body) == 0 {
		t.Error("metrics response body is empty")
	}
}

func TestRequestIDHeaderPresent(t *testing.T) {
	opts := DefaultOptions("test-service", "0")
	opts.Logger = testLogger()
	s := New(opts)
	srv := httptest.NewServer(s.Engine())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Request-ID") == "" {
		t.Errorf("X-Request-ID header missing from response")
	}
}

func TestCustomRoutesRegistered(t *testing.T) {
	opts := DefaultOptions("test-service", "0")
	opts.Logger = testLogger()
	opts.RegisterRoutes = func(api *gin.RouterGroup) {
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"pong": true})
		})
	}
	s := New(opts)
	srv := httptest.NewServer(s.Engine())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/ping")
	if err != nil {
		t.Fatalf("GET /api/v1/ping: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["pong"] != true {
		t.Errorf("pong = %v, want true", body["pong"])
	}
}

func TestResponseHelperOK(t *testing.T) {
	r := gin.New()
	r.GET("/ok", func(c *gin.Context) {
		OK(c, gin.H{"hello": "world"})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ok", nil)
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"].(float64) != 0 {
		t.Errorf("code = %v, want 0", body["code"])
	}
	if body["message"] != "ok" {
		t.Errorf("message = %q, want 'ok'", body["message"])
	}
	data := body["data"].(map[string]interface{})
	if data["hello"] != "world" {
		t.Errorf("data.hello = %v", data["hello"])
	}
}

func TestResponseHelperFail(t *testing.T) {
	r := gin.New()
	r.GET("/bad", func(c *gin.Context) {
		BadRequest(c, "invalid input")
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/bad", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if int(body["code"].(float64)) != int(config.CodeBadRequest) {
		t.Errorf("code = %v, want %d", body["code"], config.CodeBadRequest)
	}
	if body["message"] != "invalid input" {
		t.Errorf("message = %q", body["message"])
	}
}

func TestNewWithDefaults(t *testing.T) {
	opts := Options{}
	// Can't test Start without binding to real port; just ensure New doesn't panic
	s := New(opts)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.Engine() == nil {
		t.Fatal("Engine() returned nil")
	}
}

func TestTimeoutDefault(t *testing.T) {
	opts := DefaultOptions("t", "0")
	if opts.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", opts.Timeout)
	}
}
