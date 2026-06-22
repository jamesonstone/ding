// Package config loads the optional per-customer YAML metadata and the runtime
// configuration that ding reads from environment variables. Secrets never live
// in YAML; they come from the environment (or, in deployment, whatever the AWS
// console injects as Lambda environment variables).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
	"gopkg.in/yaml.v3"
)

// Config is the YAML metadata for a single customer (see customers/*.yaml). It
// holds metadata only; the database is the source of truth for invoices.
type Config struct {
	Customer struct {
		ID    string `yaml:"id"`
		Name  string `yaml:"name"`
		Email string `yaml:"email"`
	} `yaml:"customer"`
	Sender struct {
		Name  string `yaml:"name"`
		Email string `yaml:"email"`
	} `yaml:"sender"`
	Terms struct {
		NetDays int `yaml:"net_days"`
	} `yaml:"terms"`
	Email struct {
		Subject               string   `yaml:"subject"`
		Cc                    []string `yaml:"cc"`
		ReminderThresholdDays int      `yaml:"reminder_threshold_days"`
	} `yaml:"email"`
}

// LoadConfig reads and parses a customer YAML metadata file.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: read %q: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %q: %w", path, err)
	}
	if c.Customer.ID == "" {
		return Config{}, fmt.Errorf("config: %q missing customer.id", path)
	}
	return c, nil
}

// ToCustomer maps YAML metadata to a database customer for bootstrapping.
func (c Config) ToCustomer() invoice.Customer {
	return invoice.Customer{
		ID:                    c.Customer.ID,
		Name:                  c.Customer.Name,
		Email:                 c.Customer.Email,
		SenderName:            c.Sender.Name,
		SenderEmail:           c.Sender.Email,
		NetDays:               c.Terms.NetDays,
		ReminderThresholdDays: c.Email.ReminderThresholdDays,
		CreatedAt:             time.Now().UTC(),
	}
}

// Env holds runtime configuration sourced entirely from environment variables.
type Env struct {
	DBPath           string
	ResendAPIKey     string
	DiscordPublicKey string
	DiscordBotToken  string
	DiscordAppID     string
	WebhookURLs      map[string]string // customer_id -> webhook URL
	AdminUsers       []string          // Discord user IDs allowed to /seed
	LambdaMode       string            // "interactions" | "send"
}

// DefaultDBPath is used when DING_DB_PATH is unset (local development).
const DefaultDBPath = "./ding.db"

// LoadEnv reads runtime configuration from the environment. JSON-valued
// variables are tolerant of being unset (treated as empty).
func LoadEnv() (Env, error) {
	e := Env{
		DBPath:           getenv("DING_DB_PATH", DefaultDBPath),
		ResendAPIKey:     os.Getenv("RESEND_API_KEY"),
		DiscordPublicKey: os.Getenv("DISCORD_PUBLIC_KEY"),
		DiscordBotToken:  os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordAppID:     os.Getenv("DISCORD_APP_ID"),
		LambdaMode:       os.Getenv("DING_LAMBDA_MODE"),
		WebhookURLs:      map[string]string{},
	}
	if raw := strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK_URLS")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &e.WebhookURLs); err != nil {
			return Env{}, fmt.Errorf("config: DISCORD_WEBHOOK_URLS is not valid JSON object: %w", err)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DING_DISCORD_ADMIN_USERS")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &e.AdminUsers); err != nil {
			return Env{}, fmt.Errorf("config: DING_DISCORD_ADMIN_USERS is not valid JSON array: %w", err)
		}
	}
	return e, nil
}

// IsAdmin reports whether a Discord user ID is permitted to run admin commands.
func (e Env) IsAdmin(userID string) bool {
	for _, id := range e.AdminUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
