// Package sendjob is the shared, read-only "render and deliver the monthly
// summary" orchestration used by both the CLI `send` command and the monthly
// EventBridge Lambda. It performs no database writes.
package sendjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/discord"
	"github.com/jamesonstone/ding/internal/email"
	"gorm.io/gorm"
)

// Deps carries everything a send needs.
type Deps struct {
	DB          *gorm.DB
	Env         config.Env
	Now         func() time.Time
	SubjectTmpl string // optional; defaults inside email.Subject
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

// RunForCustomer renders a customer's summary and either prints it (dryRun) or
// delivers it via Resend + Discord webhook. It never writes to the database.
func RunForCustomer(ctx context.Context, d Deps, customerID string, dryRun bool, out io.Writer) error {
	cust, err := db.GetCustomer(d.DB, customerID)
	if err != nil {
		return err
	}
	invs, err := db.ListInvoices(d.DB, customerID)
	if err != nil {
		return err
	}
	today := d.now()

	html, text, err := email.Render(email.BuildData(cust, invs, today))
	if err != nil {
		return err
	}
	subject, err := email.Subject(d.SubjectTmpl, today)
	if err != nil {
		return err
	}
	embed := discord.BuildEmbed(cust, invs, today)

	if dryRun {
		enc, err := json.MarshalIndent(embed, "", "  ")
		if err != nil {
			return fmt.Errorf("sendjob: marshal embed: %w", err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "=== EMAIL: %s -> %s ===\n", subject, cust.Email)
		fmt.Fprintln(&b, "--- HTML ---")
		fmt.Fprintln(&b, html)
		fmt.Fprintln(&b, "--- TEXT ---")
		fmt.Fprintln(&b, text)
		fmt.Fprintln(&b, "--- DISCORD EMBED (JSON) ---")
		fmt.Fprintln(&b, string(enc))
		if _, err := io.WriteString(out, b.String()); err != nil {
			return fmt.Errorf("sendjob: write dry-run output: %w", err)
		}
		return nil
	}

	from := fmt.Sprintf("%s <%s>", cust.SenderName, cust.SenderEmail)
	if err := email.Send(ctx, d.Env.ResendAPIKey, email.SendParams{
		From:    from,
		To:      []string{cust.Email},
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		return err
	}
	if url := d.Env.WebhookURLs[customerID]; url != "" {
		if err := discord.PostWebhook(ctx, url, embed); err != nil {
			return err
		}
	}
	return nil
}

// RunAll sends for every customer. It attempts each one and aggregates errors so
// a single failing customer does not block the rest of the monthly run.
func RunAll(ctx context.Context, d Deps) error {
	custs, err := db.ListCustomers(d.DB)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range custs {
		if err := RunForCustomer(ctx, d, c.ID, false, io.Discard); err != nil {
			errs = append(errs, fmt.Errorf("customer %s: %w", c.ID, err))
		}
	}
	return errors.Join(errs...)
}
