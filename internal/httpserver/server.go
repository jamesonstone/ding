// Package httpserver runs ding as a long-lived HTTP server, listening for
// Discord interactions. It is suitable for EC2 or any persistent container.
package httpserver

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/discord"
)

// Start opens the database, wires the Discord handler, and listens on addr
// (e.g. ":8080"). It blocks until the server exits or a fatal error occurs.
func Start(addr string) error {
	env, err := config.LoadEnv()
	if err != nil {
		return err
	}
	gdb, err := db.Open(env.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(gdb) }()

	h := discord.Handler{Store: db.NewStore(gdb), Env: env}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /interactions", interactionsHandler(h))
	mux.HandleFunc("GET /health", healthHandler)

	log.Printf("🔔 ding http server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// interactionsHandler verifies and routes a Discord interaction received over
// HTTP, mirroring the API Gateway path used in Lambda mode.
func interactionsHandler(h discord.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Signature-Ed25519")
		ts := r.Header.Get("X-Signature-Timestamp")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = r.Body.Close() }()

		respBody, status := h.VerifyAndHandle(sig, ts, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
	}
}

// healthHandler reports basic liveness for load balancers and probes.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"status":"ok"}`)
}
