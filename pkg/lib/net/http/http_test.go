package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHTTPServerCORS(t *testing.T) {
	// Test case 1: CORS Enabled with default permissive values
	optsDefault := ServerOpts{
		Host: "localhost",
		Port: 9091,
		Cors: CORSOpts{
			Enabled: true,
		},
	}

	serverDefault := NewServer(optsDefault)
	serverDefault.engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// Preflight OPTIONS Request
	reqOpts := httptest.NewRequest(http.MethodOptions, "/test", nil)
	reqOpts.Header.Set("Origin", "http://foo.com")
	reqOpts.Header.Set("Access-Control-Request-Method", "GET")
	respOpts := httptest.NewRecorder()
	serverDefault.engine.ServeHTTP(respOpts, reqOpts)

	if respOpts.Code != http.StatusNoContent { // 204
		t.Errorf("expected preflight status 204, got %d", respOpts.Code)
	}

	if origin := respOpts.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin to be '*', got %q", origin)
	}

	// Normal GET Request
	reqGet := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqGet.Header.Set("Origin", "http://foo.com")
	respGet := httptest.NewRecorder()
	serverDefault.engine.ServeHTTP(respGet, reqGet)

	if respGet.Code != http.StatusOK {
		t.Errorf("expected GET status 200, got %d", respGet.Code)
	}

	if origin := respGet.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin to be '*', got %q", origin)
	}

	// Test case 2: CORS with custom origins
	optsCustom := ServerOpts{
		Host: "localhost",
		Port: 9092,
		Cors: CORSOpts{
			Enabled:        true,
			AllowedOrigins: []string{"http://example.com"},
		},
	}

	serverCustom := NewServer(optsCustom)
	serverCustom.engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	reqGetCustom := httptest.NewRequest(http.MethodGet, "http://localhost:9092/test", nil)
	reqGetCustom.Header.Set("Origin", "http://example.com")
	respGetCustom := httptest.NewRecorder()
	serverCustom.engine.ServeHTTP(respGetCustom, reqGetCustom)

	if origin := respGetCustom.Header().Get("Access-Control-Allow-Origin"); origin != "http://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin to be 'http://example.com', got %q", origin)
	}
}

func TestHTTPServerLogger(t *testing.T) {
	observerCore, observedLogs := observer.New(zap.InfoLevel)
	logger := zap.New(observerCore)

	opts := ServerOpts{
		Host:   "localhost",
		Port:   9093,
		Logger: logger,
	}

	server := NewServer(opts)
	server.engine.GET("/logged", func(c *gin.Context) {
		c.String(http.StatusOK, "logged response")
	})

	req := httptest.NewRequest(http.MethodGet, "/logged", nil)
	resp := httptest.NewRecorder()
	server.engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("expected GET status 200, got %d", resp.Code)
	}

	if observedLogs.Len() == 0 {
		t.Fatal("expected logs to be recorded by the zap middleware, but got 0 log entries")
	}

	// Verify that the request path was logged
	foundLoggedPath := false
	for _, entry := range observedLogs.All() {
		if entry.Message == "/logged" {
			foundLoggedPath = true
			break
		}
		// In some configurations of ginzap, the path might be in the fields
		for _, field := range entry.Context {
			if field.Key == "path" && field.String == "/logged" {
				foundLoggedPath = true
				break
			}
		}
	}

	if !foundLoggedPath {
		t.Errorf("expected log entry to contain path '/logged', but it did not. Logs: %v", observedLogs.All())
	}
}
