// Package email builds the monthly invoice summary as a multipart/alternative
// message (HTML + plaintext) and sends it through Resend. Rendering is pure and
// has no network dependency so it can be golden-tested.
package email

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
	"github.com/jamesonstone/ding/templates"
	"github.com/resend/resend-go/v2"
)

// Data is the view model passed to both templates.
type Data struct {
	CustomerName     string
	Month            string
	Year             int
	TotalOutstanding string
	SenderName       string
	Months           []MonthView
}

// MonthView is one issued-month group in the summary.
type MonthView struct {
	Label      string
	MonthTotal string
	Invoices   []InvoiceView
}

// InvoiceView is one rendered invoice row.
type InvoiceView struct {
	ID          string
	Issued      string
	Due         string
	Amount      string
	Status      string
	DaysLate    int
	Outstanding string
	Late        bool
}

const dateLayout = "2006-01-02"

var funcs = template.FuncMap{
	"upper": strings.ToUpper,
}

// BuildData assembles the view model for a customer's invoices as of `today`.
func BuildData(c invoice.Customer, invoices []invoice.Invoice, today time.Time) Data {
	d := Data{
		CustomerName:     c.Name,
		Month:            today.Month().String(),
		Year:             today.Year(),
		SenderName:       c.SenderName,
		TotalOutstanding: invoice.FormatCents(invoice.TotalOutstanding(invoices)),
	}
	for _, g := range invoice.GroupByMonth(invoices) {
		mv := MonthView{Label: g.MonthLabel(), MonthTotal: invoice.FormatCents(g.MonthTotal)}
		for _, inv := range g.Invoices {
			late := invoice.DaysLate(inv, today)
			mv.Invoices = append(mv.Invoices, InvoiceView{
				ID:          inv.ID,
				Issued:      inv.Issued.Format(dateLayout),
				Due:         inv.Due.Format(dateLayout),
				Amount:      invoice.FormatCents(inv.AmountCents),
				Status:      inv.Status,
				DaysLate:    late,
				Outstanding: invoice.FormatCents(invoice.Outstanding(inv)),
				Late:        late > 0,
			})
		}
		d.Months = append(d.Months, mv)
	}
	return d
}

// Render produces the HTML and plaintext bodies for the summary email.
func Render(d Data) (html string, text string, err error) {
	html, err = renderTemplate("email.html.tmpl", d)
	if err != nil {
		return "", "", err
	}
	text, err = renderTemplate("email.txt.tmpl", d)
	if err != nil {
		return "", "", err
	}
	return html, text, nil
}

func renderTemplate(name string, d Data) (string, error) {
	t, err := template.New(name).Funcs(funcs).ParseFS(templates.FS, name)
	if err != nil {
		return "", fmt.Errorf("email: parse %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("email: execute %s: %w", name, err)
	}
	return buf.String(), nil
}

// SendParams carries everything needed to dispatch one email through Resend.
type SendParams struct {
	From    string
	To      []string
	Cc      []string
	Subject string
	HTML    string
	Text    string
}

// Send dispatches the email via Resend. It is not exercised by unit tests
// (which cover Render); callers pass a real API key at runtime.
func Send(ctx context.Context, apiKey string, p SendParams) error {
	if apiKey == "" {
		return fmt.Errorf("email: missing RESEND_API_KEY")
	}
	client := resend.NewClient(apiKey)
	req := &resend.SendEmailRequest{
		From:    p.From,
		To:      p.To,
		Cc:      p.Cc,
		Subject: p.Subject,
		Html:    p.HTML,
		Text:    p.Text,
	}
	if _, err := client.Emails.SendWithContext(ctx, req); err != nil {
		return fmt.Errorf("email: resend send: %w", err)
	}
	return nil
}

// Subject renders the configured subject line (template with {{.Month}} and
// {{.Year}}) for the given moment, falling back to a default.
func Subject(tmpl string, today time.Time) (string, error) {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = "Invoice status — {{.Month}} {{.Year}}"
	}
	t, err := template.New("subject").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("email: parse subject: %w", err)
	}
	var buf bytes.Buffer
	data := struct {
		Month string
		Year  int
	}{today.Month().String(), today.Year()}
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("email: execute subject: %w", err)
	}
	return buf.String(), nil
}
