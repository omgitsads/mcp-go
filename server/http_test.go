package server

import "testing"

func TestStatelessHTTP(t *testing.T) {
	t.Run("Can instantiate", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		httpServer := NewStatelessHTTPServer(mcpServer, WithHTTPBasePath("/"))

		if httpServer == nil {
			t.Error("Expected httpServer to be non-nil")
		}
		if httpServer.server == nil {
			t.Error("Expected httpServer.mcpServer to be non-nil")
		}

		if httpServer.basePath != "/" {
			t.Errorf("Expected httpServer.basePath to be '/', got '%s'", httpServer.basePath)
		}
	})
}
