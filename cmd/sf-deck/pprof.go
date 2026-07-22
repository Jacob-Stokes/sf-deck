package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"strings"
	"time"
)

// startPprofFromEnv spins up a pprof HTTP server when SF_DECK_PPROF names
// a listen address, so a long-running TUI can be profiled for leaks
// without a rebuild. It's a deliberate opt-in: absent the env var this is
// a no-op. The server prints a credential-bearing URL; copy it into
// go tool pprof or curl. Leak-hunting recipe:
//
//	SF_DECK_PPROF=localhost:6060 sf-deck
//	# copy the printed http://sfdeck:<token>@... URL, then:
//	# use the app for a while, then in another shell:
//	go tool pprof 'http://sfdeck:<token>@localhost:6060/debug/pprof/heap'
//	curl -s -H 'Authorization: Bearer <token>' 'http://localhost:6060/debug/pprof/goroutine?debug=1' | head
//
// Take two heap snapshots minutes apart and diff them (`-base`) to spot
// growth. A climbing goroutine count is the clearest leak signal.
func startPprofFromEnv() {
	addr := os.Getenv("SF_DECK_PPROF")
	if addr == "" {
		return
	}
	if err := validatePprofAddr(addr); err != nil {
		fmt.Fprintln(os.Stderr, "warning: pprof disabled:", err)
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		fmt.Fprintln(os.Stderr, "warning: pprof disabled: generate access token:", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: pprof server:", err)
		return
	}
	server := &http.Server{
		Handler:           newPprofHandler(token),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "warning: pprof server:", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "pprof: serving on http://sfdeck:%s@%s/debug/pprof/\n", token, listener.Addr())
}

// newPprofHandler uses a private mux rather than http.DefaultServeMux and
// requires a per-process random credential. HTTP Basic auth keeps browser
// navigation working across the pprof index; command-line clients may instead
// send the token in an Authorization: Bearer header.
func newPprofHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			provided = strings.TrimPrefix(auth, "Bearer ")
		} else if user, password, ok := r.BasicAuth(); ok && user == "sfdeck" {
			provided = password
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "pprof access token required", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// validatePprofAddr keeps diagnostics local to the machine even though the
// endpoint is authenticated. Binding :6060 or a LAN/WAN address would expose
// an avoidable remotely reachable profiling surface.
func validatePprofAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("SF_DECK_PPROF must be a loopback host:port: %w", err)
	}
	if port == "" {
		return fmt.Errorf("SF_DECK_PPROF requires a port")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("SF_DECK_PPROF host %q is not loopback", host)
	}
	return nil
}
