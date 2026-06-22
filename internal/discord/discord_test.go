package discord

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/invoice"
)

// fakeStore is an in-memory Store for routing tests.
type fakeStore struct {
	customer invoice.Customer
	inserted []invoice.Invoice
	markPaid invoice.Invoice
	listInvs []invoice.Invoice
	getErr   error
}

func (f *fakeStore) GetCustomer(id string) (invoice.Customer, error) {
	return f.customer, f.getErr
}
func (f *fakeStore) ListInvoices(string) ([]invoice.Invoice, error) { return f.listInvs, nil }
func (f *fakeStore) InsertInvoice(inv invoice.Invoice) (invoice.Invoice, error) {
	f.inserted = append(f.inserted, inv)
	return inv, nil
}
func (f *fakeStore) MarkPaid(c, id string, d time.Time, cents int64, key string) (invoice.Invoice, bool, error) {
	return f.markPaid, false, nil
}

func newSigner(t *testing.T) (pubHex string, sign func(ts, body string) string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(pub), func(ts, body string) string {
		return hex.EncodeToString(ed25519.Sign(priv, []byte(ts+body)))
	}
}

func TestVerifyAcceptsAndRejects(t *testing.T) {
	pubHex, sign := newSigner(t)
	ts, body := "1700000000", `{"type":1}`
	if !Verify(pubHex, sign(ts, body), ts, body) {
		t.Error("valid signature rejected")
	}
	if Verify(pubHex, sign(ts, body), ts, "tampered") {
		t.Error("signature accepted for tampered body")
	}
	if Verify(pubHex, "zzzz", ts, body) {
		t.Error("malformed signature accepted")
	}
}

func TestPingReturnsPong(t *testing.T) {
	pubHex, sign := newSigner(t)
	h := Handler{Store: &fakeStore{}, Env: config.Env{DiscordPublicKey: pubHex}}
	ts, body := "1700000000", `{"type":1}`
	resp, status := h.VerifyAndHandle(sign(ts, body), ts, []byte(body))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	var r discordgo.InteractionResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatal(err)
	}
	if r.Type != discordgo.InteractionResponsePong {
		t.Errorf("response type = %d, want PONG(1)", r.Type)
	}
}

func TestInvalidSignatureUnauthorized(t *testing.T) {
	pubHex, _ := newSigner(t)
	h := Handler{Store: &fakeStore{}, Env: config.Env{DiscordPublicKey: pubHex}}
	_, status := h.VerifyAndHandle("00", "1700000000", []byte(`{"type":1}`))
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}

func TestMarkPaidReplyIsEphemeral(t *testing.T) {
	pubHex, sign := newSigner(t)
	store := &fakeStore{markPaid: invoice.Invoice{ID: "INV-001", Status: invoice.StatusPaid, AmountCents: 100000, PaidCents: 100000}}
	h := Handler{Store: store, Env: config.Env{DiscordPublicKey: pubHex}, Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	body := `{"id":"int-1","type":2,"data":{"name":"mark-paid","options":[{"name":"customer","type":3,"value":"customerx"},{"name":"invoice-id","type":3,"value":"INV-001"}]}}`
	ts := "1700000000"
	resp, status := h.VerifyAndHandle(sign(ts, body), ts, []byte(body))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	var r discordgo.InteractionResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatal(err)
	}
	if r.Type != discordgo.InteractionResponseChannelMessageWithSource {
		t.Errorf("type = %d, want 4", r.Type)
	}
	if r.Data == nil || r.Data.Flags != discordgo.MessageFlagsEphemeral {
		t.Errorf("flags = %v, want ephemeral(64)", r.Data)
	}
	if !strings.Contains(r.Data.Content, "Outstanding now $0.00") {
		t.Errorf("unexpected content: %q", r.Data.Content)
	}
}

func TestSeedAuthorizationGate(t *testing.T) {
	pubHex, sign := newSigner(t)
	store := &fakeStore{customer: invoice.Customer{ID: "customerx", NetDays: 30}}
	h := Handler{Store: store, Env: config.Env{DiscordPublicKey: pubHex, AdminUsers: []string{"admin-1"}}}
	ts := "1700000000"

	mk := func(uid string) (discordgo.InteractionResponse, []byte) {
		body := `{"id":"int-1","type":2,"member":{"user":{"id":"` + uid + `"}},"data":{"name":"seed","options":[{"name":"customer","type":3,"value":"customerx"},{"name":"id","type":3,"value":"INV-009"},{"name":"issued","type":3,"value":"2026-04-15"},{"name":"amount","type":4,"value":100000}]}}`
		resp, _ := h.VerifyAndHandle(sign(ts, body), ts, []byte(body))
		var r discordgo.InteractionResponse
		if err := json.Unmarshal(resp, &r); err != nil {
			t.Fatal(err)
		}
		return r, resp
	}

	// Non-admin is rejected and nothing is inserted.
	r, _ := mk("intruder")
	if !strings.Contains(r.Data.Content, "Not authorized") {
		t.Errorf("non-admin not rejected: %q", r.Data.Content)
	}
	if len(store.inserted) != 0 {
		t.Errorf("non-admin seed inserted %d invoices", len(store.inserted))
	}

	// Admin succeeds and the invoice is inserted with computed due date.
	r, _ = mk("admin-1")
	if !strings.Contains(r.Data.Content, "Seeded") {
		t.Errorf("admin seed failed: %q", r.Data.Content)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("admin seed inserted %d invoices, want 1", len(store.inserted))
	}
	if got := store.inserted[0].Due.Format("2006-01-02"); got != "2026-05-15" {
		t.Errorf("computed due = %s, want 2026-05-15", got)
	}
}
