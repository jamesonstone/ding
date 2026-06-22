package discord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jamesonstone/ding/internal/invoice"
)

const dateLayout = "2006-01-02"

// handle routes a verified interaction body to a command and returns the JSON
// interaction response with HTTP 200. Malformed input still returns 200 with an
// ephemeral error message, per Discord's ACK requirement.
func (h Handler) handle(body []byte) ([]byte, int) {
	var i discordgo.Interaction
	if err := json.Unmarshal(body, &i); err != nil {
		return ephemeral("Invalid request."), http.StatusOK
	}

	switch i.Type {
	case discordgo.InteractionPing:
		return mustJSON(discordgo.InteractionResponse{Type: discordgo.InteractionResponsePong}), http.StatusOK
	case discordgo.InteractionApplicationCommand:
		return h.handleCommand(&i), http.StatusOK
	default:
		return ephemeral("Unsupported interaction."), http.StatusOK
	}
}

func (h Handler) handleCommand(i *discordgo.Interaction) []byte {
	data := i.ApplicationCommandData()
	opts := optionMap(data.Options)
	switch data.Name {
	case "mark-paid":
		return h.cmdMarkPaid(i, opts)
	case "status":
		return h.cmdStatus(opts)
	case "seed":
		return h.cmdSeed(i, opts)
	default:
		return ephemeral("Unknown command.")
	}
}

func (h Handler) cmdMarkPaid(i *discordgo.Interaction, opts map[string]*discordgo.ApplicationCommandInteractionDataOption) []byte {
	customer := optString(opts, "customer")
	invoiceID := optString(opts, "invoice-id")
	if customer == "" || invoiceID == "" {
		return ephemeral("Missing required option: customer and invoice-id.")
	}
	paidDate := h.now()
	if ds := optString(opts, "date"); ds != "" {
		t, err := time.Parse(dateLayout, ds)
		if err != nil {
			return ephemeral(fmt.Sprintf("Invalid date %q (want YYYY-MM-DD).", ds))
		}
		paidDate = t
	}
	cents := optInt(opts, "cents") // 0 => paid in full
	key := invoice.IdempotencyKey(customer, i.ID, "mark-paid")
	inv, cached, err := h.Store.MarkPaid(customer, invoiceID, paidDate, cents, key)
	if err != nil {
		return ephemeral(fmt.Sprintf("Could not mark paid: %v", err))
	}
	prefix := "Marked"
	if cached {
		prefix = "Already recorded"
	}
	return ephemeral(fmt.Sprintf("%s %s/%s as %s. Outstanding now %s.",
		prefix, customer, inv.ID, inv.Status, invoice.FormatCents(invoice.Outstanding(inv))))
}

func (h Handler) cmdStatus(opts map[string]*discordgo.ApplicationCommandInteractionDataOption) []byte {
	customer := optString(opts, "customer")
	if customer == "" {
		return ephemeral("Missing required option: customer.")
	}
	invs, err := h.Store.ListInvoices(customer)
	if err != nil {
		return ephemeral(fmt.Sprintf("Could not load invoices: %v", err))
	}
	if len(invs) == 0 {
		return ephemeral(fmt.Sprintf("No invoices for %s.", customer))
	}
	var b strings.Builder
	today := h.now()
	fmt.Fprintf(&b, "**%s** — invoice status\n```\n", customer)
	for _, g := range invoice.GroupByMonth(invs) {
		fmt.Fprintf(&b, "[%s]\n", strings.ToUpper(g.MonthLabel()))
		for _, inv := range g.Invoices {
			fmt.Fprintf(&b, "  %-14s %-8s %3dd late  %s\n", inv.ID, inv.Status, invoice.DaysLate(inv, today), invoice.FormatCents(invoice.Outstanding(inv)))
		}
	}
	fmt.Fprintf(&b, "------\nTotal outstanding: %s\n```", invoice.FormatCents(invoice.TotalOutstanding(invs)))
	return ephemeral(b.String())
}

func (h Handler) cmdSeed(i *discordgo.Interaction, opts map[string]*discordgo.ApplicationCommandInteractionDataOption) []byte {
	if !h.Env.IsAdmin(userID(i)) {
		return ephemeral("Not authorized to seed invoices.")
	}
	customer := optString(opts, "customer")
	id := optString(opts, "id")
	issuedStr := optString(opts, "issued")
	amount := optInt(opts, "amount")
	if customer == "" || id == "" || issuedStr == "" || amount <= 0 {
		return ephemeral("Missing required option: customer, id, issued (YYYY-MM-DD), amount (cents > 0).")
	}
	issued, err := time.Parse(dateLayout, issuedStr)
	if err != nil {
		return ephemeral(fmt.Sprintf("Invalid issued date %q (want YYYY-MM-DD).", issuedStr))
	}
	cust, err := h.Store.GetCustomer(customer)
	if err != nil {
		return ephemeral(fmt.Sprintf("Unknown customer %q.", customer))
	}
	inv := invoice.Invoice{
		CustomerID:  customer,
		ID:          id,
		Issued:      issued,
		Due:         invoice.ComputeDue(issued, cust.NetDays),
		AmountCents: amount,
		Currency:    "USD",
		Status:      invoice.StatusUnpaid,
	}
	if _, err := h.Store.InsertInvoice(inv); err != nil {
		return ephemeral(fmt.Sprintf("Could not seed invoice: %v", err))
	}
	return ephemeral(fmt.Sprintf("Seeded %s/%s for %s due %s.", customer, id, invoice.FormatCents(amount), inv.Due.Format(dateLayout)))
}

// --- helpers ---------------------------------------------------------------

func ephemeral(content string) []byte {
	return mustJSON(discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// InteractionResponse always marshals; fall back to a minimal ack.
		return []byte(`{"type":4,"data":{"content":"internal error","flags":64}}`)
	}
	return b
}

func optionMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		m[o.Name] = o
	}
	return m
}

func optString(m map[string]*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	if o, ok := m[name]; ok && o.Value != nil {
		return strings.TrimSpace(o.StringValue())
	}
	return ""
}

func optInt(m map[string]*discordgo.ApplicationCommandInteractionDataOption, name string) int64 {
	if o, ok := m[name]; ok && o.Value != nil {
		return o.IntValue()
	}
	return 0
}

func userID(i *discordgo.Interaction) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}
