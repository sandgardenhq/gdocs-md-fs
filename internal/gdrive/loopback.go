package gdrive

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// LoopbackServer runs a temporary HTTP server on localhost to receive the OAuth
// callback redirect from Google. This replaces the deprecated OOB (out-of-band)
// copy-paste flow.
type LoopbackServer struct {
	listener net.Listener
	port     int
	codeCh   chan string
	errCh    chan error
	server   *http.Server
}

// NewLoopbackServer starts an HTTP server on a random available localhost port,
// ready to receive an OAuth callback at /callback.
func NewLoopbackServer() (*LoopbackServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("gdrive: start loopback server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &LoopbackServer{
		listener: listener,
		port:     port,
		codeCh:   codeCh,
		errCh:    errCh,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("gdrive: oauth error: %s", errMsg)
			fmt.Fprint(w, "Authorization failed. You can close this tab.")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("gdrive: no authorization code in callback")
			fmt.Fprint(w, "Authorization failed: no code received. You can close this tab.")
			return
		}

		codeCh <- code
		fmt.Fprint(w, "Authorization successful! You can close this tab and return to the terminal.")
	})

	srv.server = &http.Server{Handler: mux}
	go func() { _ = srv.server.Serve(listener) }()

	return srv, nil
}

// RedirectURL returns the full callback URL to use as the OAuth redirect URI.
func (s *LoopbackServer) RedirectURL() string {
	return fmt.Sprintf("http://localhost:%d/callback", s.port)
}

// Port returns the port the server is listening on.
func (s *LoopbackServer) Port() int {
	return s.port
}

// WaitForCode blocks until the OAuth callback delivers an authorization code
// or the context is cancelled.
func (s *LoopbackServer) WaitForCode(ctx context.Context) (string, error) {
	select {
	case code := <-s.codeCh:
		return code, nil
	case err := <-s.errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close shuts down the HTTP server.
func (s *LoopbackServer) Close() {
	_ = s.server.Close()
}
