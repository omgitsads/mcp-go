package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// StatelessHTTPContextFunc is a function that takes an existing context and returns
// a potentially modified context.
// This can be used to inject context values from environment variables,
// for example.
type StatelessHTTPContextFunc func(ctx context.Context) context.Context

// StatelessHTTPServer wraps a MCPServer and handles StatelessHTTP communication.
// It provides a simple way to create command-line MCP servers that
// communicate via standard input/output streams using JSON-RPC messages.
type StatelessHTTPServer struct {
	server      *MCPServer
	basePath    string
	errLogger   *log.Logger
	contextFunc StatelessHTTPContextFunc

	srv *http.Server

	mu sync.RWMutex
}

// StatelessHTTPOption defines a function type for configuring StatelessHTTPServer
type StatelessHTTPOption func(*StatelessHTTPServer)

// WithContextFunc sets a function that will be called to customise the context
// to the server. Note that the StatelessHTTP server uses the same context for all requests,
// so this function will only be called once per server instance.
func WithStatelessHTTPContextFunc(fn StatelessHTTPContextFunc) StatelessHTTPOption {
	return func(s *StatelessHTTPServer) {
		s.contextFunc = fn
	}
}

func WithHTTPBasePath(basePath string) StatelessHTTPOption {
	return func(s *StatelessHTTPServer) {
		s.basePath = basePath
	}
}

// NewStatelessHTTPServer creates a new StatelessHTTP server wrapper around an MCPServer.
// It initializes the server with a default error logger that discards all output.
func NewStatelessHTTPServer(server *MCPServer, opts ...StatelessHTTPOption) *StatelessHTTPServer {
	svr := &StatelessHTTPServer{
		server:   server,
		basePath: "/",
		errLogger: log.New(
			os.Stderr,
			"",
			log.LstdFlags,
		), // Default to discarding logs
	}

	for _, opt := range opts {
		opt(svr)
	}

	return svr
}

// SetErrorLogger configures where error messages from the StatelessHTTPServer are logged.
// The provided logger will receive all error messages generated during server operation.
func (s *StatelessHTTPServer) SetErrorLogger(logger *log.Logger) {
	s.errLogger = logger
}

// SetContextFunc sets a function that will be called to customise the context
// to the server. Note that the StatelessHTTP server uses the same context for all requests,
// so this function will only be called once per server instance.
func (s *StatelessHTTPServer) SetContextFunc(fn StatelessHTTPContextFunc) {
	s.contextFunc = fn
}

// Start begins serving SSE connections on the specified address.
// It sets up HTTP handlers for SSE and message endpoints.
func (s *StatelessHTTPServer) Start(addr string) error {
	s.mu.Lock()
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s,
	}
	s.mu.Unlock()

	return s.srv.ListenAndServe()
}

// writeJSONRPCError writes a JSON-RPC error response with the given error details.
func (s *StatelessHTTPServer) writeJSONRPCError(
	w http.ResponseWriter,
	id interface{},
	code int,
	message string,
) {
	response := createErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}

// processMessage handles a single JSON-RPC message and writes the response.
// It parses the message, processes it through the wrapped MCPServer, and writes any response.
// Returns an error if there are issues with message processing or response writing.
func (s *StatelessHTTPServer) processMessage(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Check if Accept header includes either application/json and text/event-stream
	acceptHeader := r.Header.Get("Accept")
	if !strings.Contains(acceptHeader, "application/json") && !strings.Contains(acceptHeader, "text/event-stream") {
		http.Error(w, "Not Acceptable: Client must accept both application/json and text/event-stream", http.StatusNotAcceptable)
		return
	}

	// Check if Content-Type header is application/json
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type: Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	if r.Method == http.MethodGet {
		// Return 405 as we don't support Streaming Yet
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if the request is a POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	// Parse message as raw JSON
	var rawMessage json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawMessage); err != nil {
		s.writeJSONRPCError(w, nil, mcp.PARSE_ERROR, "Parse error")
		return
	}
	// Handle the message using the wrapped server
	response := s.server.HandleMessage(r.Context(), rawMessage)

	if response != nil {
		// send http response
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			s.errLogger.Printf("Error writing response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// for notifications, just send 202 accepted with no body
		w.WriteHeader(http.StatusAccepted)
	}
}

// ServeHTTP implements the http.Handler interface.
func (s *StatelessHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == s.basePath {
		s.processMessage(w, r)
		return
	}

	http.NotFound(w, r)
}
