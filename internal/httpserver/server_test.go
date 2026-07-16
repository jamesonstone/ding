package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/discord"
)

// TestHealthHandler verifies the liveness endpoint returns 200 and ok json.
func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %q, want %q", got, `{"status":"ok"}`)
	}
}

// TestInteractionsHandlerRejectsInvalidSignature ensures an unsigned request is
// rejected with 401, mirroring the Lambda interactions path.
func TestInteractionsHandlerRejectsInvalidSignature(t *testing.T) {
	h := discord.Handler{Env: config.Env{DiscordPublicKey: ""}}
	req := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(`{"type":1}`))
	rec := httptest.NewRecorder()

	interactionsHandler(h)(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
