package http

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"heddle/pkg/runtime/interaction"
)

// Request representa a requisição HTTP enviada ao callback Heddle
type Request struct {
	Method  string
	Path    string
	Body    string
	Headers map[string]string
}

// Response representa a resposta retornada pelo callback Heddle
type Response struct {
	Status  int
	Body    string
	Headers map[string]string
}

type HTTPServer struct {
	Host     string
	Port     int
	engine   *gin.Engine
	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
}

type CORSOpts struct {
	Enabled          bool
	AllowCredentials bool
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
}

type ServerOpts struct {
	Host   string
	Port   int
	Cors   CORSOpts
	Logger *zap.Logger
}

func NewServer(opts ServerOpts) *HTTPServer {
	host := ""
	port := 8080
	if opts.Host != "" {
		host = opts.Host
	}
	if opts.Port != port {
		port = opts.Port
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	logger := opts.Logger
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			logger = zap.NewNop()
		}
	}

	engine.Use(ginzap.Ginzap(logger, time.RFC3339, true))
	engine.Use(ginzap.RecoveryWithZap(logger, true))

	if opts.Cors.Enabled {
		engine.Use(corsMiddleware(opts.Cors))
	}

	s := &HTTPServer{
		Host:   host,
		Port:   port,
		engine: engine,
	}
	_ = s.Init()
	return s
}

func corsMiddleware(opts CORSOpts) gin.HandlerFunc {
	config := cors.Config{
		AllowCredentials: opts.AllowCredentials,
	}

	allowAllOrigins := false
	if len(opts.AllowedOrigins) == 0 {
		allowAllOrigins = true
	} else {
		if slices.Contains(opts.AllowedOrigins, "*") {
			allowAllOrigins = true
		}
	}

	if allowAllOrigins {
		config.AllowAllOrigins = true
	} else {
		config.AllowOrigins = opts.AllowedOrigins
	}

	if len(opts.AllowedMethods) > 0 {
		config.AllowMethods = opts.AllowedMethods
	} else {
		config.AllowMethods = []string{"POST", "OPTIONS", "GET", "PUT", "DELETE"}
	}

	if len(opts.AllowedHeaders) > 0 {
		config.AllowHeaders = opts.AllowedHeaders
	} else {
		config.AllowHeaders = []string{
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"X-CSRF-Token",
			"Authorization",
			"accept",
			"origin",
			"Cache-Control",
			"X-Requested-With",
		}
	}

	return cors.New(config)
}

type HTTPTag string

const (
	TagOK HTTPTag = "ok"
)

func (s *HTTPServer) handleRoute(method string, path string, yield func(req Request) Response) {
	s.engine.Handle(method, path, func(c *gin.Context) {
		bodyBytes, _ := io.ReadAll(c.Request.Body)

		headers := make(map[string]string)
		for k, v := range c.Request.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		req := Request{
			Method:  c.Request.Method,
			Path:    c.Request.URL.Path,
			Body:    string(bodyBytes),
			Headers: headers,
		}

		resp := yield(req)
		for k, v := range resp.Headers {
			c.Writer.Header().Set(k, v)
		}
		c.Writer.WriteHeader(resp.Status)
		_, _ = c.Writer.Write([]byte(resp.Body))
	})
}

func (s *HTTPServer) Get(path string) (interaction.ReqReply[HTTPTag, Request, Response], error) {
	return func(yield func(req Request) Response) {
		s.handleRoute(http.MethodGet, path, yield)
	}, nil
}

func (s *HTTPServer) Post(path string) (interaction.ReqReply[HTTPTag, Request, Response], error) {
	return func(yield func(req Request) Response) {
		s.handleRoute(http.MethodPost, path, yield)
	}, nil
}

func (s *HTTPServer) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = l
	s.Port = l.Addr().(*net.TCPAddr).Port

	s.server = &http.Server{Handler: s.engine}
	go func() {
		err := s.server.Serve(l)
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP SERVE ERROR: %v\n", err)
		}
	}()
	return nil
}
