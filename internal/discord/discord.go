// Package discord handles Discord slash-command interactions (Ed25519
// verification, routing, ephemeral replies) and builds the monthly summary
// embed posted via webhook. Data access is abstracted behind Store so the
// routing logic is unit-testable without a database.
package discord

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/invoice"
)

// Store is the subset of data access the interaction handler needs.
type Store interface {
	GetCustomer(id string) (invoice.Customer, error)
	ListInvoices(customerID string) ([]invoice.Invoice, error)
	InsertInvoice(inv invoice.Invoice) (invoice.Invoice, error)
	MarkPaid(customerID, invoiceID string, paidDate time.Time, paidCents int64, idemKey string) (inv invoice.Invoice, cached bool, err error)
}

// Handler routes verified Discord interactions against a Store.
type Handler struct {
	Store Store
	Env   config.Env
	Now   func() time.Time
}

// now returns the handler's clock (defaulting to time.Now in UTC).
func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().UTC()
}

// Verify reports whether a Discord interaction signature is valid for the given
// Ed25519 public key (hex), using the timestamp+body message Discord signs.
// This is the same construction as discordgo.VerifyInteraction, adapted to the
// API Gateway header/body shape.
func Verify(publicKeyHex, signature, timestamp, body string) bool {
	key, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(key), []byte(timestamp+body), sig)
}

// VerifyAndHandle verifies the signature then routes the interaction. An invalid
// signature yields 401; everything else returns 200 with a Discord interaction
// response (Discord requires an ACK even for user-facing errors).
func (h Handler) VerifyAndHandle(signature, timestamp string, body []byte) (response []byte, status int) {
	if !Verify(h.Env.DiscordPublicKey, signature, timestamp, string(body)) {
		return []byte("invalid request signature"), http.StatusUnauthorized
	}
	return h.handle(body)
}

// BuildEmbed builds the monthly summary embed: customer name, total
// outstanding, and the top three latest invoices.
func BuildEmbed(c invoice.Customer, invoices []invoice.Invoice, today time.Time) *discordgo.MessageEmbed {
	total := invoice.TotalOutstanding(invoices)
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🔔 %s — invoice status", c.Name),
		Description: fmt.Sprintf("Total outstanding: **%s**", invoice.FormatCents(total)),
		Color:       0xd9534f,
		Timestamp:   today.Format(time.RFC3339),
	}
	for _, inv := range topLate(invoices, today, 3) {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   inv.ID,
			Value:  fmt.Sprintf("%s outstanding · %d days late", invoice.FormatCents(invoice.Outstanding(inv)), invoice.DaysLate(inv, today)),
			Inline: false,
		})
	}
	return embed
}

// PostWebhook posts the embed to a Discord webhook URL.
func PostWebhook(ctx context.Context, webhookURL string, embed *discordgo.MessageEmbed) error {
	if webhookURL == "" {
		return fmt.Errorf("discord: empty webhook URL")
	}
	payload := map[string]any{"embeds": []*discordgo.MessageEmbed{embed}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("discord: build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: post webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// topLate returns up to n invoices with the highest current lateness (>0).
func topLate(invoices []invoice.Invoice, today time.Time, n int) []invoice.Invoice {
	type scored struct {
		inv  invoice.Invoice
		late int
	}
	var late []scored
	for _, inv := range invoices {
		if d := invoice.DaysLate(inv, today); d > 0 {
			late = append(late, scored{inv, d})
		}
	}
	// simple selection sort by lateness desc; lists are tiny
	for i := 0; i < len(late); i++ {
		max := i
		for j := i + 1; j < len(late); j++ {
			if late[j].late > late[max].late {
				max = j
			}
		}
		late[i], late[max] = late[max], late[i]
	}
	if len(late) > n {
		late = late[:n]
	}
	out := make([]invoice.Invoice, len(late))
	for i, s := range late {
		out[i] = s.inv
	}
	return out
}
