---
kit_metadata_version: 1
artifact: spec
workflow_version: 2
phase: deliver
feature:
  id: 0001
  slug: init-project
  dir: 0001-init-project
references:
  - id: feature-notes
    name: Feature notes
    type: notes
    target: docs/notes/0001-init-project
    relation: informs
    read_policy: conditional
    used_for: optional pre-brainstorm research input
    status: optional
  - id: github-pr-delivery
    name: GitHub PR delivery ruleset
    type: ruleset
    target: docs/references/rules/github-pr-delivery.md
    relation: governs
    read_policy: required-at-delivery
    used_for: delivery hard-gate contract (issue/branch/commit/PR rules)
    status: pending
  - id: kit-go-reference
    name: kit sibling repo (Go patterns)
    type: code
    target: ../kit
    relation: pattern-source
    read_policy: conditional
    used_for: package layout, Makefile targets, cobra wiring conventions
    status: verified
  - id: constitution
    name: Project constitution
    type: doc
    target: docs/CONSTITUTION.md
    relation: constrains
    read_policy: conditional
    used_for: project invariants and code-file size guidance
    status: verified
delivery_intent: issue_branch_pr_later
---
# SPEC

## THESIS

# 🔔 ding — Specification

> Get rich or die tryin'.

Invoice tracking and automated payment reminders via persistent Discord bot. PostgreSQL state backend, Resend transactional email, Lambda + EventBridge for scheduling and interactions.

---

## 1. Architecture overview

**Three core flows:**

1. **Monthly send** — EventBridge cron (1st of month) → Lambda → query Postgres → render email → Resend → Discord embed post.
2. **Discord interactions** — API Gateway receives slash commands/buttons → Lambda → query/mutate Postgres → reply to Discord.
3. **Local CLI** — binary for testing, seeding, and status checks. Queries same Postgres.

**Infrastructure:**
- **Compute:** AWS Lambda (API Gateway for interactions, EventBridge for cron).
- **Data:** PostgreSQL (Aurora Serverless v2) + RDS Proxy for connection pooling.
- **Secrets:** AWS Secrets Manager (Resend API key, Discord webhook URLs, bot token, public key).
- **Logs:** CloudWatch Logs (structured, searchable).
- **Email:** Resend API (transactional).
- **Chat:** Discord (persistent bot via webhook, slash commands ephemeral).

## 2. Stack

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.22 | single static binary, matches existing toolchain |
| State | PostgreSQL (Aurora Serverless v2) | ACID, RDS Proxy pooling, no file I/O |
| Config | YAML (metadata only) | readable, version-controlled |
| Email | Resend API | best deliverability + Go SDK |
| Compute | AWS Lambda | serverless, no infra, cheap at scale |
| Scheduling | EventBridge cron | AWS-native, reliable, free tier covers it |
| Chat | Discord bot (slash commands) | persistent interactions, no listening gateway needed (webhook model) |
| Secrets | AWS Secrets Manager | managed rotation, Lambda IAM integration |

## 3. Data model

### 3.1 Config — `customers/customerx-ding.yaml` (static, metadata only)

```yaml
customer:
  id: customerx
  name: Customer X
  email: customerx@example.com
sender:
  name: Jameson Stone
  email: jameson@stone.tc
terms:
  net_days: 30
email:
  subject: "Invoice status — {{.Month}} {{.Year}}"
  cc: []
  reminder_threshold_days: 0
```

Config is metadata only; all mutable state lives in Postgres. Secrets (API keys, webhooks) are never in YAML — stored in AWS Secrets Manager.

### 3.2 State — PostgreSQL (Aurora Serverless v2)

All mutable state in Postgres. Customers and invoices tables (see §15 for full schema). Lambda queries on every request; no in-memory caching.

### 3.3 Go types

```go
type Customer struct {
	ID                    string    `db:"id"`
	Name                  string    `db:"name"`
	Email                 string    `db:"email"`
	SenderName            string    `db:"sender_name"`
	SenderEmail           string    `db:"sender_email"`
	NetDays               int       `db:"net_days"`
	ReminderThresholdDays int       `db:"reminder_threshold_days"`
	DiscordWebhookSecret  string    `db:"discord_webhook_secret"` // secret name, not URL
	CreatedAt             time.Time `db:"created_at"`
}

type Invoice struct {
	ID             string     `db:"id"`
	CustomerID     string     `db:"customer_id"`
	Issued         time.Time  `db:"issued"`
	Due            time.Time  `db:"due"`
	AmountCents    int64      `db:"amount_cents"`
	Currency       string     `db:"currency"`
	Status         string     `db:"status"` // unpaid|partial|paid
	PaidDate       *time.Time `db:"paid_date"`
	PaidCents      int64      `db:"paid_cents"`
	IdempotencyKey *string    `db:"idempotency_key"` // hash of (customer_id, action_id, action_type)
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
}
```

Use `pgx` or `database/sql` with `sqlc` for type-safe queries.

### 3.4 Secrets management

**Locally (CLI dev):** Env vars.
```bash
export DATABASE_URL="postgres://user:password@localhost:5432/ding"
export RESEND_API_KEY="re_..."
export DISCORD_WEBHOOK_CUSTOMERX="https://discord.com/api/webhooks/..."
```

**Lambda:** AWS Secrets Manager. Retrieve at startup, cache for 15min.
- Secret name: `ding/resend-api-key` → string.
- Secret name: `ding/discord-webhook-urls` → JSON `{customer_id: url}`.
- Secret name: `ding/discord-bot-token` → string (for registering commands in CI).
- Secret name: `ding/discord-public-key` → string (for interaction verification).

Lambda retrieves via `aws-sdk-go-v2/service/secretsmanager`. Never log secrets. Never pass through YAML config.

## 4. Core logic (validated by simulation)

```
daysLate(inv, today):
  if status == paid && paid_date != nil: return max(0, paid_date - due)   # historical
  if status != paid:                     return max(0, today - due)        # current
  return 0

outstanding(inv):
  return status == paid ? 0 : amount_cents - paid_cents
```

Sim output for 4 representative invoices (today = 2026-06-22):

| Invoice | Issued | Due | Amount | Status | Late | Outstanding |
|---|---|---|---|---|---|---|
| INV-2026-004 | 2026-03-20 | 2026-04-19 | $1,500.00 | partial | 64d | $750.00 |
| INV-2026-001 | 2026-04-15 | 2026-05-15 | $2,500.00 | unpaid | 38d | $2,500.00 |
| INV-2026-002 | 2026-05-10 | 2026-06-09 | $1,800.00 | paid | 0d | $0.00 |
| INV-2026-003 | 2026-06-01 | 2026-07-01 | $3,200.00 | unpaid | 0d | $3,200.00 |

**Total outstanding: $6,450.00.** Partial nets correctly; not-yet-due not flagged late; paid-on-time zeroed.

## 5. CLI surface

All commands require `DATABASE_URL` env var. Config YAML is optional (holds metadata; database is source of truth for invoices).

| Command | Effect |
|---|---|
| `ding migrate` | run GORM migrations (one-time setup, run before first Lambda deploy) |
| `ding send <customer-id> [--dry-run]` | query invoices for customer, render, send email (skip with `--dry-run`), post Discord (skip with `--dry-run`), no DB writes (read-only) |
| `ding mark-paid <customer-id> --id INV-... --date YYYY-MM-DD [--cents N]` | UPDATE invoices SET status, paid_date, paid_cents with idempotency check |
| `ding status <customer-id>` | SELECT invoices, print grouped table to stdout |
| `ding validate <customer-id>` | verify customer exists in DB, no invoices with invalid due dates |
| `ding seed <customer-id> --id ... --issued ... --amount ...` | INSERT invoice |

**Dry-run mode:**
```bash
./bin/ding send customerx --dry-run
```

Outputs: rendered HTML email to stdout, Discord embed JSON to stdout. No Resend call, no Discord webhook. Useful for testing templates locally.

Fail-closed: any DB connection error, missing customer, malformed date → non-zero exit, no email sent.

## 6. Discord bot setup

**Slash command registration** (one-time):
```bash
curl -X POST https://discord.com/api/v10/applications/{APP_ID}/commands \
  -H "Authorization: Bot {TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name": "mark-paid", "description": "Mark invoice paid", "options": [{"name": "invoice-id", "type": 3, "required": true}, {"name": "date", "type": 3, "required": true}, {"name": "cents", "type": 4}]}'
```
Repeat for `/status` and `/seed`. Store `APP_ID`, `TOKEN` in Secrets Manager.

**Interaction verification** (Lambda):
```go
valid := discordgo.VerifyInteraction(discord.PublicKey, sig, ts, body)
if !valid { return http.StatusUnauthorized, nil }
```
Discord public key from Secrets Manager. Headers: `X-Signature-Ed25519`, `X-Signature-Timestamp`.

**Interaction format:**
- Request: type 2, data with name/options, member/channel fields.
- Response: type 4 (CHANNEL_MESSAGE_WITH_SOURCE), flags 64 (ephemeral).

## 7. Lambda + EventBridge architecture

### 7.0 Email template structure (Gmail-safe)

Rendered as **multipart/alternative** (HTML + plaintext fallback).

**HTML template** (`templates/email.html.tmpl`):
- Inline CSS only; Gmail strips `<style>` tags.
- Table-based layout with columns: Invoice, Issued, Due, Amount, Status, Days Late, Outstanding.
- Grouped by month with subtotals. Late invoices (DaysLate > 0) in red (#d9534f) with left border.
- Grand total row (bold, background tint) at bottom.

**Plaintext template** (`templates/email.txt.tmpl`):
- Tab-separated, month headers as `[MARCH 2026]`, dashes for separation, grand total on last line.

**Template variables:**
```
{{.CustomerName}}, {{.Month}}, {{.Year}}, {{.TotalOutstanding}}
{{.InvoicesByMonth}} → {{.Month}}/{{.Year}}, {{.Invoices}} → {{.ID}}, {{.Issued}}, {{.Due}}, {{.Amount}}, {{.Status}}, {{.DaysLate}}, {{.Outstanding}}
{{.MonthTotal}}, {{.SenderName}}
```

### 7.1 Monthly send (EventBridge cron → Lambda)

EventBridge rule fires on `cron(0 13 1 * * ? *)` (1st day of each month, 13:00 UTC). Invokes a Lambda function that:
- For each customer in the DB, calls send logic.
- Queries invoices for that customer.
- Renders email, calls Resend, posts Discord.
- No DB writes; read-only.

Lambda connects to Aurora via RDS Proxy. IAM auth or password in Lambda env.

### 7.2 Discord interactions (API Gateway → Lambda)

POST `/interactions` endpoint (API Gateway). Receives Discord interaction payloads (slash commands, button clicks).

Routes:
- `/mark-paid <invoice-id> --date YYYY-MM-DD [--cents N]` → UPDATE invoices, reply with new outstanding.
- `/status` → SELECT invoices, return formatted table as Discord ephemeral message.
- Button clicks on monthly post → marshal slash command and call handler above.

All interactions query/mutate the Postgres database. Lambda auth via IAM (preferred) or password secret.

### 7.3 GitHub Actions (optional, for CI/deploy only)

`.github/workflows/build.yml`: on push to main, build + test the binary. SAM handles Lambda packaging and deployment.

State is in Postgres; GHA does **not** commit state. Deployment is via SAM (see §15-17 for SAM template and AWS setup).

## 8. Lambda, Discord, errors, and idempotency

### 8.1 Error handling + observability

Log to **CloudWatch Logs** with structured format: `action=X customer=Y error=Z`.

**Failure modes:**
- **Resend down:** Log error, reply Discord "Email service unavailable", non-zero exit (no retry).
- **Postgres down:** Log error, reply "Database connection failed", non-zero exit (EventBridge or manual retry).
- **Malformed interaction:** Log error + payload, reply "Invalid request", HTTP 200 (Discord ACK).
- **Idempotency collision:** Log warning, return cached result (user sees same confirmation; safe).

**Optional CloudWatch alarms:** Error rate > 10% on send, p99 latency > 2s on mark-paid, error rate > 20% on validation.

### 8.2 Discord interactions

Persistent bot, slash-command driven. No one-way webhook; full interaction model.

**Commands:**
- `/mark-paid <invoice-id> [--date YYYY-MM-DD] [--cents N]` → mutates state, replies with updated outstanding total.
- `/status` → ephemeral reply; shows current outstanding by invoice, grouped by month.
- `/seed <id> <issued-date> <amount>` → adds an invoice to state. Requires user ID in approved list (configured in Lambda env). **Authorization:** managed via simple list in Secrets Manager (`ding/discord-admin-users` = JSON array of Discord user IDs).

**Monthly post:** EventBridge-triggered Lambda posts an embed (customer name, total outstanding, top 3 late, link to `/mark-paid` command). Embeds have a "Mark paid" button → slash command shortcut.

**Interactions** are ephemeral (user-only) or silent (instant confirmation). No persistent connection needed; API Gateway → Lambda webhook.

### 8.3 Bootstrap — first customer / bulk import

**Single customer (CLI):**
```bash
./bin/ding seed customerx --id INV-001 --issued 2026-04-15 --amount 250000
./bin/ding seed customerx --id INV-002 --issued 2026-05-10 --amount 180000
```

**Bulk import (SQL):**
```sql
INSERT INTO customers (...) VALUES ('customerx', 'Customer X', 'customerx@example.com', ...);
INSERT INTO invoices (id, customer_id, issued, due, amount_cents, currency, status)
VALUES ('INV-2026-001', 'customerx', '2026-04-15', '2026-05-15', 250000, 'USD', 'unpaid');
```

### 8.4 Idempotency implementation

Discord may retry interactions within 3 seconds. Prevent double-charges via idempotency keys.

**Generate key:**
```go
import "crypto/md5"
import "encoding/hex"

func idempotencyKey(customerID, interactionID, action string) string {
	hash := md5.Sum([]byte(customerID + "|" + interactionID + "|" + action))
	return hex.EncodeToString(hash[:])
}

// Usage: key := idempotencyKey(cid, interaction.ID, "mark-paid")
```

**Check before mutate:**
```go
// Before UPDATE: check if this key exists
var existing Invoice
result := db.Where("idempotency_key = ?", key).First(&existing)
if result.Error == nil {
	// Already processed; return cached result
	return existing.Status, existing.PaidDate, existing.Outstanding
}

// Not yet processed; proceed with UPDATE
// After UPDATE, store the key in the same row
db.Model(&invoice).Update("idempotency_key", key)
```

This ensures Discord retries return identical results without side effects.

## 9. Repo layout

```
ding/
├── cmd/ding/main.go              # entry point; wire everything
├── internal/
│   ├── config/config.go          # YAML load (metadata only)
│   ├── db/db.go                  # Postgres conn pool, queries
│   ├── invoice/invoice.go        # Invoice type, daysLate, outstanding logic
│   ├── email/email.go            # Resend client, template render
│   ├── discord/discord.go        # webhook post
│   └── lambda/lambda.go          # Lambda event handler, context wiring
├── migrations/001_init.sql       # schema: customers, invoices
├── customers/customerx-ding.yaml # metadata only (moved to DB)
├── templates/{email.html.tmpl,email.txt.tmpl}
├── .github/workflows/{ding-build.yml}
├── Makefile
├── README.md
└── go.mod
```

Match **package organization, Makefile targets, and README format** from `event-collector` and `kit`. Specifics below.

## 10. Development — Makefile, README, package org

### 10.1 Makefile

Match `event-collector` and `kit` patterns. Targets: `build`, `test`, `lint`, `fmt`, `clean`, `help`.

### 10.2 README.md

Match format from `event-collector` and `kit`: one-liner + description, quick start, config, usage, architecture, development, license. Terse, no badges.

### 10.3 Package organization (internal/)

- `config/` — YAML load, no DB access.
- `db/` — Postgres pool, queries, invoice/customer rows.
- `invoice/` — Types, pure functions (daysLate, outstanding, groupByMonth).
- `email/` — Resend setup, template render, send.
- `discord/` — Webhook post.
- `lambda/` — Event unmarshal, context wire, error handling.

Follow error wrapping, logging, idioms from existing codebases.

## 11. Build order

1. **AWS infra setup** (manual or IaC/SAM): Aurora Serverless, RDS Proxy, Secrets Manager, Lambda, API Gateway, EventBridge, IAM roles.
2. **GORM migrations**: `internal/db/migrations.go`, runner function, manual test against local Postgres.
3. Types + DB connection pool setup + config load (YAML for metadata only).
4. `daysLate` / `outstanding` logic with table tests.
5. Email templates + Resend send.
6. `send` command (query invoices, compute, render, email, Discord).
7. `mark-paid` command + idempotency key check + `seed` command (INSERT/UPDATE queries).
8. `status` command (SELECT, format output).
9. `ding` binary (cmd/main.go, cobra subcommands: migrate, send, mark-paid, status, validate, seed).
10. Lambda entry point (internal/lambda/handler.go, Discord interaction unmarshaling, DB queries, response routing).
11. API Gateway integration (Terraform/SAM, Discord URL verification, interaction payload routing).
12. EventBridge integration (cron rule, Lambda target).
13. Local test harness (makefile targets, mock DB or local Postgres for dev, Discord interaction mocks).

## 12. Go module dependencies

```
module github.com/jamesonstone/ding

go 1.22

require (
	github.com/resend/resend-go v0.8.0         // Resend email API
	github.com/bwmarrin/discordgo v0.27.1      // Discord bot library
	github.com/aws/aws-lambda-go v1.41.0       // Lambda runtime
	github.com/aws/aws-sdk-go-v2 v1.24.0       // AWS SDK (Secrets Manager, etc)
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.27.0
	gorm.io/gorm v1.25.5                       // ORM + migrations
	gorm.io/driver/postgres v1.5.7             // Postgres driver
	github.com/spf13/cobra v1.8.0              // CLI framework
	github.com/google/uuid v1.6.0              // UUID generation (idempotency keys)
)
```

**Install:**
```bash
go mod init github.com/jamesonstone/ding
go get github.com/resend/resend-go
go get github.com/bwmarrin/discordgo
go get github.com/aws/aws-lambda-go
go get github.com/aws/aws-sdk-go-v2
go get gorm.io/gorm gorm.io/driver/postgres
go get github.com/spf13/cobra
go get github.com/google/uuid
```

## 13. Local CLI usage

**Setup:**
```bash
make build
export DATABASE_URL="postgres://testuser:testpass@localhost:5432/ding_dev"
./bin/ding validate customerx
```

**Common flows:**
```bash
./bin/ding send customerx --dry-run          # test (no email/Discord)
./bin/ding seed customerx --id INV-001 --issued 2026-04-15 --amount 250000
./bin/ding status customerx
./bin/ding mark-paid customerx --id INV-001 --date 2026-06-22 [--cents 50000]
```

**Lambda:** Handler unmarshals Discord JSON, calls same logic, replies. No CLI invocation.

## 14. Lambda environment variables

| Variable | Source | Purpose |
|---|---|---|
| `DATABASE_URL` | Set in SAM template | Postgres RDS Proxy endpoint |
| `AWS_REGION` | Auto | Set by Lambda runtime |

**DATABASE_URL format:** `postgres://user:pass@host:5432/ding` or RDS Proxy: `postgres://user:pass@ding-proxy.abc12345.region.rds.amazonaws.com:5432/ding`

**Secrets (retrieved at startup, not env vars):**
```go
func init() {
	cfg, _ := config.LoadDefaultConfig(context.Background())
	smClient := secretsmanager.NewFromConfig(cfg)
	
	resendKeyResult, _ := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("ding/resend-api-key"),
	})
	resendKey = *resendKeyResult.SecretString
	
	webhookResult, _ := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("ding/discord-webhook-urls"),
	})
	json.Unmarshal([]byte(*webhookResult.SecretString), &discordWebhooks)
}
```

Secrets in Secrets Manager: `ding/resend-api-key`, `ding/discord-webhook-urls`, `ding/discord-admin-users`, `ding/discord-public-key`.

## 15. GORM migrations

Migrations run **once during deployment**, not on every Lambda invocation.

**Pattern:** Track applied migrations in migrations table. For each migration, check if applied; if not, run it and record.

```go
type Migration struct {
	Version int `gorm:"primaryKey"`; Name string; AppliedAt time.Time
}

func RunMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&Migration{}); err != nil { return err }
	migrations := []struct{ version int; name string; fn func(*gorm.DB) error }{
		{1, "create_customers_invoices", migrateInitial},
		{2, "add_idempotency_keys", migrateIdempotency},
	}
	for _, m := range migrations {
		if err := db.Where("version = ?", m.version).First(&Migration{}).Error; err == nil { continue }
		if err := m.fn(db); err != nil { return fmt.Errorf("migration %d: %w", m.version, err) }
		db.Create(&Migration{Version: m.version, Name: m.name, AppliedAt: time.Now()})
	}
	return nil
}

func migrateInitial(db *gorm.DB) error { return db.AutoMigrate(&Customer{}, &Invoice{}) }
func migrateIdempotency(db *gorm.DB) error { return db.Migrator().AddColumn(&Invoice{}, "idempotency_key") }
```

**Deployment:** Run migrations locally before pushing Lambda. Lambda does **not** run migrations on startup (fail-fast if schema missing).

**Schema:**

```sql
create table customers (
  id text primary key,
  name text not null,
  email text not null,
  sender_name text not null,
  sender_email text not null,
  net_days int default 30,
  reminder_threshold_days int default 0,
  discord_webhook_secret text,
  created_at timestamp default now()
);

create table invoices (
  id text primary key,
  customer_id text references customers(id) on delete cascade,
  issued date not null,
  due date not null,
  amount_cents bigint not null,
  currency text default 'USD',
  status text check (status in ('unpaid', 'partial', 'paid')),
  paid_date date,
  paid_cents bigint default 0,
  idempotency_key text unique,
  created_at timestamp default now(),
  updated_at timestamp default now(),
  unique(customer_id, id)
);

create index idx_invoices_customer_status on invoices(customer_id, status);
create index idx_invoices_customer_due on invoices(customer_id, due);
create index idx_invoices_idempotency on invoices(idempotency_key);
```

**Idempotency:** Discord interaction handler generates `idempotency_key` = hash of (customer_id, interaction_id, action). Check if this key exists before mutation. If resubmitted, return cached result instead of re-executing. Prevents double-charging if Discord retries.

## 16. SAM template (infrastructure as code)

Minimal `template.yaml`:
```yaml
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2013-12-31
Description: ding — invoice reminder service

Globals:
  Function:
    Runtime: provided.al2
    Environment:
      Variables:
        DATABASE_URL: !Sub '${DBUrl}'

Parameters:
  DBUrl:
    Type: String
    Description: Postgres RDS Proxy endpoint

Resources:
  DingInteractionFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: ding-interactions
      Handler: bootstrap
      CodeUri: ./cmd/ding/
      Timeout: 30
      MemorySize: 512
      Policies:
        - Version: '2012-10-17'
          Statement:
            - Effect: Allow
              Action: 'secretsmanager:GetSecretValue'
              Resource: !Sub 'arn:aws:secretsmanager:${AWS::Region}:${AWS::AccountId}:secret:ding/*'
            - Effect: Allow
              Action: 'logs:*'
              Resource: !Sub 'arn:aws:logs:${AWS::Region}:${AWS::AccountId}:log-group:/aws/lambda/*'
      Events:
        DiscordInteraction:
          Type: HttpApi
          Properties:
            Path: /interactions
            Method: POST

  DingSendFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: ding-send
      Handler: bootstrap
      CodeUri: ./cmd/ding/
      Timeout: 60
      MemorySize: 512
      Policies: [ same as above ]
      Events:
        MonthlyCron:
          Type: Schedule
          Properties:
            Schedule: 'cron(0 13 1 * ? *)'
```

Deploy: `sam build && sam deploy --guided`

## 17. AWS / Lambda deployment

**Required resources:** Aurora Serverless v2, RDS Proxy, Secrets Manager, Lambda (512MB, 30s/60s timeout), API Gateway (HTTP), EventBridge (cron), IAM role.

**Lambda IAM policy:**
- `secretsmanager:GetSecretValue` for `ding/*` secrets.
- `logs:*` for CloudWatch.
- `rds-db:connect` if using IAM auth.

**Deploy via GitHub Actions + SAM:**
```yaml
name: deploy-ding
on: { push: { branches: [main] } }
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - run: make build && go test ./... -v
      - run: |
          export DATABASE_URL="${{ secrets.DATABASE_URL }}"
          go run ./cmd/ding migrate
      - uses: aws-actions/setup-sam@v2
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1
      - run: sam build && sam deploy --no-confirm-changeset --no-fail-on-empty-changeset
```

**Local dev:**
```bash
export DATABASE_URL="postgres://user:pass@localhost/ding"
make build && ./bin/ding send customerx --dry-run
```

## CONTEXT

Greenfield bootstrap. The repository contains only Kit scaffolding (docs tree, agent
instruction files, `.kit.yaml`) and a one-line `README.md`. There is no `go.mod` and no
Go source yet. This feature (`0001-init-project`) builds the entire `ding` application
described in the Thesis from scratch.

Key repo-grounded findings:

- Local toolchain is Go 1.26.1; the Thesis specifies `go 1.22` for the module.
- Docker daemon is **up**, which enables disposable-Postgres integration validation.
- Reference repos named in Thesis §10 exist locally:
  - `../kit` is the real **Go** reference (cobra, `go 1.25.5`, `Makefile` targets, thin
    `cmd/<bin>/main.go` → `pkg/cli`, domain-split `internal/`).
  - `../event-collector` is a **Python** project (pyproject/uv), so its Go package
    organization does not apply; only README/Makefile *format* is loosely relevant.
- Delivery is gated: `.kit.yaml` registers `github-pr-delivery`, `safety-guardrails`,
  `work-lane-gating`, `kit-capabilities-usage` rulesets; confidence bar is 95%.

### Dirty Worktree And Ownership

`git status --short` at start:

```
 M .gitignore
?? .coderabbit.yaml
?? .github/
?? .kit.yaml
?? AGENTS.md
?? CLAUDE.md
?? docs/
```

Classification: **all user/Kit-owned scaffolding**, none is supervisor implementation
work. `.gitignore` diff only adds Kit-generated ignore patterns (`.kit/runs/`, etc.). All
other paths are untracked Kit scaffolding (the spec workflow itself plus agent docs). I
will not stage, reformat, claim, or summarize these as my work. My implementation adds new
files (Go sources, `Makefile`, workflow, templates, etc.). Existing files I modified:
`README.md` (replaced the one-line stub) and `.gitignore` (appended `/bin/` + `*.db`
artifact ignores below the Kit scaffolding edits — additive, no Kit lines changed).

### Source Map

| ID | Source | Selector | Claim / Fact | Used For | Maps To | Status |
|----|--------|----------|--------------|----------|---------|--------|
| SRC-001 | user decision (Batch 1 Q1) | — | Course change: data store is **SQLite, no external DB** (supersedes all Thesis Postgres/Aurora/RDS-Proxy text). ORM/driver pending Q-B2 | Data layer | REQ-DATA | Resolved (driver pending) |
| SRC-002 | repo root | `find . -name '*.go'`→∅; no `go.mod` | Greenfield; full bootstrap is in scope | Scope | all ACs | Verified |
| SRC-003 | `../kit` | `go.mod`, `Makefile`, `cmd/kit/main.go`, `internal/*` | Go reference: cobra, Makefile `build/test/lint(golangci-lint)/fmt/vet/clean/install/tidy/all`, thin `main`→`pkg/cli`, domain `internal/` | Match existing patterns (Thesis §10) | REQ-LAYOUT | Verified |
| SRC-004 | `../event-collector` | `pyproject.toml`, `uv.lock`, `src/` | Is a **Python** project; Go package org N/A; corrects Thesis §10 | Pattern matching | REQ-LAYOUT | Verified (corrects thesis) |
| SRC-005 | local shell | `go version`→1.26.1; `docker info`→up | Toolchain present; Docker enables disposable Postgres integration tests | Validation strategy | Q4 → VALIDATION | Verified |
| SRC-006 | `docs/agents/GUARDRAILS.md` | "GitHub Delivery Hard Gate"; PR title `<type>(<issue>): <gitmoji> <title>` | Delivery requires repo-local contract; no git/GitHub mutation during clarify | Delivery gate | Delivery Decision | Verified |
| SRC-007 | `.kit.yaml` | `registry.artifacts`, `goal_percentage: 95`, `feature_naming.numeric_width: 4` | Delivery rulesets to load at delivery gate; 95% confidence bar | Readiness + delivery | Delivery Decision | Verified |
| SRC-008 | `git status --short`, `git diff .gitignore` | — | All worktree changes are Kit/user-owned scaffolding, not impl work | Ownership gate | Dirty Worktree | Verified |
| SRC-009 | `docs/specs/0001-init-project/SPEC.md` | §16, §7.1, §7.2 | One binary (`CodeUri ./cmd/ding`) must serve 2 Lambda functions (interactions, send) **and** the cobra CLI | Runtime entrypoint routing | Q5 → REQ-ENTRY | Needs default confirmation |
| SRC-010 | `docs/specs/0001-init-project/SPEC.md` | §8.4 | Idempotency key uses `crypto/md5` | Hash choice (lint) | Q6 default | Low-stakes |
| SRC-011 | user decision (Batch 1 Q2) | — | **No IaC this pass**; AWS via console; env-var runtime config; Discord registration is a doc'd manual step (supersedes §11/§16/§17 SAM text) | Scope boundary | REQ-SCOPE | Resolved |
| SRC-012 | AWS Lambda runtime behavior (known constraint) | `/tmp` semantics | Lambda `/tmp` is **ephemeral + per-execution-environment** → SQLite there cannot be the persistent system of record | Runtime persistence model + docs caveat | REQ-PERSIST, AC-018 | Resolved (documented caveat) |
| SRC-013 | `.envrc`, `.env` | `dotenv_if_exists`; `.env` empty | direnv-based env-var config already wired; `.env.example` is the natural delivery vehicle | Env-var config | REQ-ENV | Verified |
| SRC-014 | `.github/` | `pull_request_template.md`, `copilot-instructions.md`; no `workflows/` | PR template exists (delivery gate); CI workflow dir must be created if CI kept | CI scope + delivery | Q-B2-4 | Verified |

## CLARIFICATIONS

### Batch 1 — status: resolved

- **Q1 — OVERRIDE.** Course change: use **SQLite**, run on the container-runtime
  filesystem (deployed) or local filesystem (dev). **No external database** (no
  Postgres/Aurora/RDS Proxy). This removes the entire Postgres/Aurora/RDS-Proxy stack.
- **Q2 — OVERRIDE.** **No IaC this pass** (no SAM `template.yaml`, no Terraform). User
  will create all AWS resources manually via the console. Runtime config is **env-var
  driven**; this feature must define/document the complete set of required environment
  variables.
- **Q3 — ACCEPT.** One cohesive PR.
- **Q4 — ACCEPT** (mechanism updated by Q1): validation floor = pure-logic unit tests +
  **SQLite integration tests against a temp DB file** (Docker no longer needed) + `ding
  send --dry-run`. No live Resend/Discord/AWS calls.
- **Q5 — ACCEPT.** Single `cmd/ding` binary; detect Lambda via `AWS_LAMBDA_RUNTIME_API`;
  route the two Lambda functions via `DING_LAMBDA_MODE=interactions|send`; else cobra CLI.
- **Q6 — ACCEPT.** `crypto/sha256` idempotency hash; `go 1.22` in `go.mod`;
  `reminder_threshold_days` persisted as metadata only (not wired into send/late logic).

### Batch 2 — status: resolved (all defaults accepted)

- **Q1 — ACCEPT.** App is storage-path-agnostic via `DING_DB_PATH`; deployed persistence
  (persistent single-writer volume or Lambda+EFS) is the user's console responsibility.
  Bare Lambda `/tmp` documented as ephemeral/unsuitable as system of record (SRC-012).
- **Q2 — ACCEPT.** GORM + pure-Go `glebarez/sqlite`; preserves `AutoMigrate`, static binary.
- **Q3 — ACCEPT.** All secrets/config from env vars; drop `secretsmanager` SDK; keep
  `aws-lambda-go`.
- **Q4 — ACCEPT.** Minimal build+test CI at `.github/workflows/ding-build.yml`; no deploy.
- **Q5 — ACCEPT.** Env-var inventory adopted (see REQUIREMENTS REQ-ENV) via `.env.example`.

Confidence: **96%**. Unresolved questions: **0**. All readiness gates evaluated below.

## REQUIREMENTS

Functional + non-functional requirements (revised for the SQLite / no-IaC / env-var
architecture; Thesis Postgres/Aurora/SAM/Secrets-Manager text is superseded where noted).

- **REQ-DATA**: State persists in **SQLite** at `DING_DB_PATH` via GORM + `glebarez/sqlite`
  (pure-Go). Schema = `customers`, `invoices`, `migrations` with the constraints/indexes of
  Thesis §15 (status check, `unique(customer_id,id)`, unique `idempotency_key`, the three
  indexes). No external database.
- **REQ-MIGRATE**: `ding migrate` runs a versioned migration runner (migrations table +
  `AutoMigrate`); idempotent (re-run is a no-op, exit 0).
- **REQ-LOGIC**: Pure functions `DaysLate`, `Outstanding`, `GroupByMonth` match Thesis §4
  exactly (paid→historical lateness from paid_date; unpaid→today−due, floored at 0;
  outstanding = 0 when paid else amount−paid).
- **REQ-CMDS**: Cobra commands `migrate`, `send`, `mark-paid`, `status`, `validate`, `seed`
  with Thesis §5 flags; fail-closed (DB error / missing customer / malformed date →
  non-zero exit, no email).
- **REQ-SEND**: `send` is read-only: query → compute → render email (HTML+text) → Resend →
  Discord embed. `--dry-run` prints HTML + Discord embed JSON to stdout, no external calls,
  no DB writes.
- **REQ-MARKPAID**: `mark-paid` updates `status`/`paid_date`/`paid_cents`; `partial` when
  `paid_cents < amount`, `paid` when `>=`; honors idempotency (REQ-IDEMPOTENT).
- **REQ-SEED**: `seed` inserts an invoice; `due = issued + net_days`; default status
  `unpaid`, currency `USD`.
- **REQ-STATUS**: `status` selects invoices and prints a month-grouped table with per-row
  DaysLate/Outstanding and a grand total.
- **REQ-VALIDATE**: `validate` exits 0 only when the customer exists and due dates are
  valid; non-zero otherwise.
- **REQ-EMAIL**: HTML template = inline CSS only (no `<style>`), table layout, month
  grouping + subtotals, late rows (DaysLate>0) red `#d9534f`, bold grand-total row;
  plaintext alt = `[MONTH YEAR]` headers + grand total; template vars per Thesis §7.0.
- **REQ-DISCORD**: Ed25519 verification (`discordgo.VerifyInteraction`); PING(1)→PONG(1);
  invalid signature → unauthorized; slash routing for `/mark-paid`, `/status`, `/seed`;
  responses ephemeral (type 4, flags 64); `/seed` gated by `DING_DISCORD_ADMIN_USERS`.
- **REQ-LAMBDA**: Same binary serves both Lambda functions; `interactions` = API Gateway
  proxy → verify → route; `send` = iterate customers → send logic.
- **REQ-ENTRY**: `cmd/ding/main.go` runs the Lambda handler when `AWS_LAMBDA_RUNTIME_API`
  is set (function chosen by `DING_LAMBDA_MODE`), else the cobra CLI.
- **REQ-IDEMPOTENT**: key = `hex(sha256(customerID|interactionID|action))`; check-before-
  mutate; on collision return the cached result with no second mutation.
- **REQ-ENV**: All secrets/config read from env vars (no Secrets Manager). Inventory:
  `DING_DB_PATH`, `RESEND_API_KEY`, `DISCORD_PUBLIC_KEY`, `DISCORD_BOT_TOKEN`,
  `DISCORD_APP_ID`, `DISCORD_WEBHOOK_URLS` (JSON `{customer_id:url}`),
  `DING_DISCORD_ADMIN_USERS` (JSON array), `DING_LAMBDA_MODE`. Documented in `.env.example`
  + README.
- **REQ-PERSIST**: Storage-path-agnostic; docs state deployed runtime must provide
  persistent single-writer storage and that Lambda `/tmp` is ephemeral (SRC-012).
- **REQ-LAYOUT**: Go layout adapted from `../kit`: thin `cmd/ding/main.go`, domain
  `internal/*`, Makefile targets `build test lint fmt vet clean tidy help`, module
  `github.com/jamesonstone/ding`, `go 1.22`.
- **REQ-CI**: `.github/workflows/ding-build.yml` runs build + test + vet on push/PR; no
  deploy/IaC.
- **REQ-DOCS**: README in kit format + a deployment runbook covering manual AWS console
  setup, the persistence caveat, Discord command registration, and the env var table.

Non-goals (this feature): IaC/SAM/Terraform; live AWS provisioning; live Resend/Discord
sends in tests; AWS Secrets Manager; multi-currency math beyond stored `currency`;
wiring `reminder_threshold_days` into send/late logic.

## ASSUMPTIONS

Accepted (Batch 1):

- A1 (revised): Data store is **SQLite** at a configurable path; **no external database**.
- A2 (revised): **No IaC this pass**; AWS created manually via console by the user;
  runtime config and secrets delivered via **environment variables** documented by this
  feature. Discord command registration remains a documented manual step.
- A3: All CLI commands, the Lambda handlers (both modes), email rendering, Discord
  verification, migrations, env-var config, and docs land in one cohesive PR.
- A4 (revised): Validation floor = pure-logic unit tests + SQLite integration tests
  against a temp DB file + `ding send --dry-run`; no live Resend/Discord/AWS calls.
- A5: Single `cmd/ding` binary; Lambda detected via `AWS_LAMBDA_RUNTIME_API`; functions
  routed via `DING_LAMBDA_MODE`.
- A6: Idempotency key uses `crypto/sha256`.
- A7: `go.mod` declares `go 1.22`.
- A8: `reminder_threshold_days` persisted as metadata only.

Accepted (Batch 2):

- A9: SQLite driver/ORM = **GORM + `glebarez/sqlite`** (pure-Go, preserves AutoMigrate,
  static binary).
- A10: App is storage-path-agnostic via `DING_DB_PATH`; deployed persistence is the user's
  console responsibility. Bare Lambda `/tmp` ephemeral caveat documented (SRC-012).
- A11: Secrets/config from **env vars**; drop AWS Secrets Manager + secretsmanager SDK;
  keep `aws-lambda-go`.
- A12: Minimal **build + test** GitHub Actions workflow (no deploy/IaC).
- A13: Env-var inventory delivered via documented `.env.example` (direnv `.envrc`).

Design adaptations (supervisor decisions, low-risk, recorded for traceability):

- D1: Shared domain structs (`Customer`, `Invoice`) live in `internal/invoice` with GORM
  tags + pure logic; `internal/db` imports them (avoids an import cycle / duplicate
  structs vs the literal §9/§10.3 split). `Migration` model lives in `internal/db`.
- D2: Cobra command definitions live in `internal/cli`; `cmd/ding/main.go` stays thin and
  only routes CLI-vs-Lambda (matches `../kit` thin-main pattern + 300-line guideline).
- D3: Thesis §11/§16/§17 SAM/Aurora/Secrets-Manager content is superseded by the SQLite +
  no-IaC + env-var decisions and is retained only as historical context.
- D4: Email templates live at the repo-root `templates/` (per §9) but are exposed through a
  tiny `package templates` with `//go:embed`, so the single binary is self-contained
  (Go embed requires co-located files).
- D5: `resend-go/v2` pinned to **v2.10.0** (go 1.19) because v2.28.0 declares `go 1.23`,
  which would force the module's go directive above the accepted `go 1.22` (A7). Pin holds
  AC-003. The Resend send API used (`Emails.SendWithContext`) is identical across versions.

## ACCEPTANCE CRITERIA

| ID | Criterion (binary-verifiable) |
|----|-------------------------------|
| AC-001 | `make build` produces `bin/ding`; `go build ./...` exits 0 |
| AC-002 | `gofmt -l` reports no files and `go vet ./...` exits 0 (golangci-lint run if installed) |
| AC-003 | `go.mod` = module `github.com/jamesonstone/ding`, `go 1.22`; requires gorm, glebarez/sqlite, resend-go, discordgo, aws-lambda-go, cobra; contains **no** postgres driver and **no** aws secretsmanager |
| AC-004 | Invoice unit tests reproduce Thesis §4 exactly: DaysLate = {64,38,0,0}, Outstanding(cents) = {75000,250000,0,320000}, total outstanding = 645000 (\$6,450.00) for today=2026-06-22 |
| AC-005 | `ding migrate` on a fresh temp DB creates customers/invoices/migrations tables + the 3 §15 indexes and is idempotent (2nd run exit 0, no change) |
| AC-006 | `ding seed` inserts an invoice with `due = issued + net_days`, status `unpaid`; appears in `ding status` |
| AC-007 | `ding status <customer>` prints invoices grouped by month with per-row DaysLate/Outstanding and a grand total |
| AC-008 | `ding mark-paid` sets `partial` when paid_cents<amount and `paid` when ≥; updated outstanding reflected |
| AC-009 | `ding validate` exits 0 for a valid customer and non-zero for a missing customer (fail-closed) |
| AC-010 | `ding send <customer> --dry-run` prints rendered HTML + Discord embed JSON, makes no Resend/Discord call, writes nothing to the DB, exits 0 |
| AC-011 | Rendered HTML has inline CSS only (no `<style>`), month subtotals, late rows colored `#d9534f`, a grand-total row; plaintext alt has `[MONTH YEAR]` headers + grand total |
| AC-012 | Idempotency key = `hex(sha256(customerID\|interactionID\|action))`, deterministic (unit-tested); a repeated key returns cached result with no second mutation |
| AC-013 | Interaction handler: PING(1)→PONG(1); valid Ed25519 signature accepted, invalid rejected as unauthorized; command responses are type 4 with flags 64 |
| AC-014 | `/seed` interaction rejected for a user absent from `DING_DISCORD_ADMIN_USERS`, accepted for an admin |
| AC-015 | Entry routing: with `AWS_LAMBDA_RUNTIME_API` set the selector chooses the Lambda handler per `DING_LAMBDA_MODE`; unset → cobra CLI (unit-tested selector) |
| AC-016 | Code reads config only from env vars (no secretsmanager symbol anywhere); all REQ-ENV vars present in `.env.example` and README |
| AC-017 | `.github/workflows/ding-build.yml` exists, parses as YAML, runs build+test+vet, has no deploy step |
| AC-018 | README (kit format) + runbook document manual AWS console setup, the SQLite persistence caveat (Lambda /tmp ephemeral), Discord command registration, and the env var table |

## IMPLEMENTATION PLAN

Approach: build bottom-up from shared domain types, validating each layer, then wire the
CLI and Lambda entrypoints, then CI + docs. Greenfield — every file is new except the
`README.md` stub (overwritten).

Predicted touched files (all new unless noted):

- `go.mod`, `go.sum`, `Makefile`, `.env.example`, `customers/customerx-ding.yaml`
- `cmd/ding/main.go` (CLI-vs-Lambda routing only)
- `internal/invoice/{invoice.go,invoice_test.go}` (structs+GORM tags, DaysLate/Outstanding/GroupByMonth, idempotency key)
- `internal/db/{db.go,migrate.go,queries.go,db_test.go}` (open SQLite, migration runner, CRUD, integration tests)
- `internal/config/config.go` (YAML metadata + env-var config)
- `internal/email/{email.go,email_test.go}`, `templates/{email.html.tmpl,email.txt.tmpl}`
- `internal/discord/{discord.go,discord_test.go}` (verify, route, embed, webhook post)
- `internal/lambda/{lambda.go,lambda_test.go}` (interaction + send handlers, mode selector)
- `internal/cli/*.go` (cobra root + 6 subcommands)
- `.github/workflows/ding-build.yml`
- `README.md` (overwrite stub), `docs/references/deployment.md` (runbook)

Sequencing & rollback: foundation (`go.mod` + `internal/invoice`) first; then `db` and
`email` in parallel; then `discord`+`lambda`; then `cli`+`config`+`cmd`; then CI+docs.
Validate after each layer. Rollback is trivial (greenfield): `git clean -fd` new files +
`git checkout -- README.md .gitignore`. No git/GitHub mutation until the delivery gate.

### Agent Team Plan

- **Supervisor (me)**: owns SPEC.md, integration, validation synthesis, delivery gating.
- **Lane A — Foundation** (serialized first): `go.mod`, `internal/invoice` (+tests),
  `Makefile`. Dependency root for all other lanes.
- **Lane B — Persistence**: `internal/db` (+integration tests). After A.
- **Lane C — Email**: `internal/email` + templates (+render tests). After A; parallel w/ B.
- **Lane D — Discord+Lambda**: `internal/discord`, `internal/lambda` (+tests). After B & C.
- **Lane E — CLI/Config/CI/Docs**: `internal/cli`, `internal/config`, `cmd/ding/main.go`,
  workflow, README, runbook, `.env.example`. Integrates everything; last.
- **Verification lane (read-only)**: audit Source Map ↔ diff ↔ tests ↔ docs before reflect.
- Overlap risk: low across lanes (distinct packages); the only shared surface is the
  `internal/invoice` structs (owned by Lane A, frozen before B–E). Max concurrency 2
  (B‖C). **Execution note**: lanes are logical groupings; the supervisor executes them
  directly (no subagents spawned unless the user requests parallel agents).

## TASK CHECKLIST

Status: `[ ]` pending · `[~]` in progress · `[x]` done · `[!]` blocked

- [x] T1 (Lane A) `go mod` + deps (resend pinned v2.10.0 to hold `go 1.22`); `Makefile` pending in T-build — AC-003 ✓
- [x] T2 (Lane A) `internal/invoice` structs (GORM tags) + DaysLate/Outstanding/GroupByMonth + idempotency key + tests — AC-004 ✓, AC-012 ✓
- [x] T3 (Lane B) `internal/db` open(SQLite)+migration runner+CRUD + integration tests — AC-005 ✓, AC-006 ✓, AC-008 ✓
- [x] T4 (Lane C) `internal/email` render + `templates/*` (root + embed pkg) + render tests — AC-011 ✓
- [x] T5 (Lane C) `internal/config` YAML metadata + env-var config + `customers/customerx-ding.yaml` + `.env.example` — AC-016 ✓
- [x] T6 (Lane D) `internal/discord` verify+route+embed+webhook + tests — AC-013 ✓, AC-014 ✓
- [x] T7 (Lane D) `internal/lambda` interaction+send handlers + mode selector + tests; `internal/sendjob` shared send — AC-015 ✓
- [x] T8 (Lane E) `internal/cli` cobra commands (migrate/send/mark-paid/status/validate/seed) — AC-006..AC-010 ✓
- [x] T9 (Lane E) `cmd/ding/main.go` CLI-vs-Lambda routing — AC-015 ✓
- [x] T10 (Lane E) `Makefile` + `.github/workflows/ding-build.yml` — AC-001 ✓, AC-002 ✓, AC-017 ✓
- [x] T11 (Lane E) `README.md` (kit format) + `docs/references/deployment.md` runbook — AC-018 ✓, AC-016 ✓
- [x] T12 (Verify) `make build` + `go test ./...` + `go vet` + `gofmt` + `golangci-lint` (0 issues) + CLI smoke + dry-run read-only check — AC-001 ✓, AC-002 ✓, AC-010 ✓

## VALIDATION MAP

| AC | Method / command | Result |
|----|------------------|--------|
| AC-001 | `make build` → `bin/ding` (22 MB); `go build ./...` | PASS |
| AC-002 | `gofmt -l` empty; `go vet ./...` clean; `golangci-lint run ./...` → **0 issues** | PASS |
| AC-003 | `go.mod`: module `github.com/jamesonstone/ding`, `go 1.22`; `grep -ci postgres`=0, `grep -ci secretsmanager`=0 | PASS |
| AC-004 | `go test ./internal/invoice/...` — DaysLate {64,38,0,0}, Outstanding {75000,250000,0,320000}, total 645000 | PASS |
| AC-005 | `go test ./internal/db/...` — temp DB creates 3 tables + 3 §15 indexes, 2nd migrate no-op (count=1) | PASS |
| AC-006 | db test seed→status round-trip + CLI smoke (`seed` due=2026-05-15) | PASS |
| AC-007 | CLI smoke `status customerx` — month groups + subtotals + grand total | PASS |
| AC-008 | `go test ./internal/db/...` — partial(60000)→paid(0) transitions; idempotent replay cached | PASS |
| AC-009 | CLI smoke `validate customerx` exit 0; `validate ghostco` exit 1 | PASS |
| AC-010 | CLI smoke `send --dry-run` — HTML+text+embed JSON to stdout; DB sha unchanged before/after; no creds set, exit 0 | PASS |
| AC-011 | `go test ./internal/email/...` — no `<style>`, `#d9534f`, subtotals, `[MARCH 2026]`, `TOTAL OUTSTANDING` | PASS |
| AC-012 | `go test ./internal/invoice/...` — sha256 hex (len 64), deterministic, distinct per interaction | PASS |
| AC-013 | `go test ./internal/discord/...` — Verify accept/reject; PING→PONG(1); invalid sig→401; flags 64 | PASS |
| AC-014 | `go test ./internal/discord/...` — non-admin `/seed` rejected (0 inserts), admin succeeds | PASS |
| AC-015 | `go test ./internal/lambda/...` — SelectRoute matrix (CLI/interactions/send/unknown) | PASS |
| AC-016 | source `grep` secretsmanager/aws-sdk-go = 0; 8/8 env vars present in `.env.example` + README | PASS |
| AC-017 | `ding-build.yml` parses (job `build`), runs vet/test/build, deploy refs = 0 | PASS |
| AC-018 | README + `docs/references/deployment.md` cover console setup, persistence caveat, Discord registration, env table | PASS |

## REFLECTION NOTES

Implementation matches the thesis as revised by the SQLite / no-IaC / env-var course
change. Review findings:

- **Thesis alignment**: all six CLI commands, both Lambda modes, email render, Discord
  verification + routing, idempotency, and the §4 lateness/outstanding math are present and
  validated. Superseded thesis content (Postgres/Aurora/RDS-Proxy/SAM/Secrets-Manager) was
  intentionally dropped per Batch 1/2 decisions (recorded D3).
- **Scope creep**: none. No abstractions beyond what the lanes required. The only additions
  beyond the literal §9 layout are the `templates` embed package (D4), `internal/sendjob`
  (shared send used by both CLI and Lambda, avoids duplication), and `internal/cli` (D2).
- **Dead code / surface**: none found. `internal/db.Store` is consumed by `discord` and the
  lambda wiring; all exported symbols are used. `ListCustomers` is used by `sendjob.RunAll`.
- **Error handling**: commands fail closed (non-zero exit on DB error / missing customer /
  bad date); dry-run is read-only (verified by DB checksum). Monthly `RunAll` aggregates
  per-customer errors so one failure does not abort the batch.
- **Lint/format**: `golangci-lint` clean (0 issues), `gofmt` clean, `go vet` clean.

Remaining risks / accepted limitations:

- **R1 (persistence)**: SQLite on AWS Lambda `/tmp` is not a valid system of record
  (ephemeral, per-instance). Mitigation: documented requirement for a persistent
  single-writer volume (EFS/EBS) in README + runbook (SRC-012, AC-018). Not a code defect;
  an operational constraint the user owns (no IaC this pass).
- **R2 (go directive)**: `go.mod` is `go 1.22` per A7, held by pinning `resend-go/v2 v2.10.0`
  (D5). If the Resend API surface is later upgraded, the go directive may need to move to
  1.23+.
- **R3 (Discord buttons)**: the monthly embed posts fields but no interactive "Mark paid"
  button (thesis §8.2 nice-to-have); slash commands cover the same action. Documented as a
  future enhancement, not in this feature's acceptance criteria.
- **R4 (Discord `customer` option)**: commands take an explicit `customer` option (thesis
  examples omitted it) because state is multi-customer (recorded as runbook §5).

No correctness gap found; no rework routed back.

## DOCUMENTATION UPDATES

| Doc | Change | Status |
|-----|--------|--------|
| `README.md` | Replaced 2-line stub with full kit-format README (quick start, env config table, usage, architecture, persistence caveat, dev) | Updated |
| `docs/references/deployment.md` | New manual-AWS runbook (build, persistence model, env vars, console setup, Discord registration, verify) | Added |
| `.env.example` | New — full env var inventory consumed by direnv `.envrc` | Added |
| `customers/customerx-ding.yaml` | New — sample customer metadata | Added |
| `.gitignore` | Appended `/bin/` + `*.db*` artifact ignores (additive) | Updated |
| `docs/PROJECT_PROGRESS_SUMMARY.md` | Updated init-project phase + summary | Updated |
| `docs/CONSTITUTION.md` | Left unchanged — no new project invariants introduced by this feature | No-op (intentional) |
| `AGENTS.md` / `CLAUDE.md` / `.github/copilot-instructions.md` | Left unchanged — routing tables unaffected | No-op (intentional) |

## DELIVERY DECISION

- **Intent**: `issue_branch_pr_later` — user will create the issue, branch, and PR later
  using Kit-managed repository rules.
- **Hard-gate status**: NOT executed. No Git or GitHub mutation has been performed (no
  `git add`, commit, branch, push, issue, or PR). The worktree holds the new
  implementation files plus the pre-existing Kit scaffolding, all unstaged.
- **When delivery is requested**, load `docs/references/rules/github-pr-delivery.md` +
  `docs/agents/GUARDRAILS.md`, run delivery recon, and present the Delivery Contract
  (PR title format `<type>(<issue>): <gitmoji> <title>`) for approval before any mutation.
- **Rollback** (greenfield): `git clean -fd` the new files and `git checkout -- README.md
  .gitignore` to return to the initial scaffolding state.

## EVIDENCE

Validation run on 2026-06-22, Go 1.26.1 toolchain (module targets `go 1.22`):

- `make build` → `bin/ding` produced; `go build ./...` clean.
- `go test ./...` → all packages PASS (invoice, db, email, discord, lambda).
- `go vet ./...` clean; `gofmt -l` empty; `golangci-lint run ./...` → **0 issues**.
- CLI smoke (temp `DING_DB_PATH`): migrate → seed×2 → validate(0)/validate-missing(1) →
  mark-paid(partial $750) → status (month groups + grand total $3,250) → send --dry-run
  (HTML+text+embed JSON, **DB sha256 identical before/after** = read-only).
- Dependency hygiene: `go.mod` has no postgres driver, no secretsmanager; source has no
  `secretsmanager`/`aws-sdk-go` references.

### Acceptance Coverage Audit

| AC | Impl evidence (files) | Validation | Docs | Verifier | Gap |
|----|-----------------------|------------|------|----------|-----|
| AC-001 | `Makefile`, `cmd/ding`, all pkgs | `make build` | README dev | self | PASS |
| AC-002 | whole tree | gofmt/vet/golangci-lint 0 | — | self | PASS |
| AC-003 | `go.mod` | inspect + grep | — | self | PASS |
| AC-004 | `internal/invoice/invoice.go` | `invoice_test.go` | — | self | PASS |
| AC-005 | `internal/db/migrate.go` | `db_test.go` | — | self | PASS |
| AC-006 | `internal/cli/mutate.go`, `internal/db/queries.go` | db test + CLI smoke | README usage | self | PASS |
| AC-007 | `internal/cli/read.go` | CLI smoke | README usage | self | PASS |
| AC-008 | `internal/db/queries.go` | `db_test.go` | — | self | PASS |
| AC-009 | `internal/cli/read.go`, `cli.go` | CLI smoke exit codes | README | self | PASS |
| AC-010 | `internal/sendjob/sendjob.go`, `internal/cli/send.go` | CLI smoke + sha check | README | self | PASS |
| AC-011 | `templates/*`, `internal/email/email.go` | `email_test.go` | — | self | PASS |
| AC-012 | `internal/invoice/invoice.go` | `invoice_test.go` | — | self | PASS |
| AC-013 | `internal/discord/discord.go`,`interactions.go` | `discord_test.go` | runbook | self | PASS |
| AC-014 | `internal/discord/interactions.go` | `discord_test.go` | runbook | self | PASS |
| AC-015 | `internal/lambda/lambda.go`, `cmd/ding/main.go` | `lambda_test.go` | README arch | self | PASS |
| AC-016 | `internal/config/config.go`, `.env.example` | grep + presence | README env table | self | PASS |
| AC-017 | `.github/workflows/ding-build.yml` | yaml parse + grep | — | self | PASS |
| AC-018 | `README.md`, `docs/references/deployment.md` | doc inspection | n/a | self | PASS |

All 18 criteria PASS. No failed, blocked, or disputed rows. Verifier column is `self`
(supervisor); no separate read-only verification lane was spawned (single-lane execution,
see Agent Team note) — the audit was performed against the actual diff, tests, and runtime
output.
