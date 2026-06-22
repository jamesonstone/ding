# ding

🔔 Get Invoices Paid

Invoice tracking and automated monthly payment reminders. A single Go binary
serves a local CLI and two AWS Lambda functions (Discord slash-command
interactions and a monthly EventBridge send job). State lives in a single
**SQLite** file — there is no external database. Email goes out through Resend;
the monthly summary is also posted to Discord.

## Quick Start

```bash
make build
export DING_DB_PATH="./ding.db"          # local SQLite file
./bin/ding migrate                        # create the schema (run once)
./bin/ding seed customerx --id INV-2026-001 --issued 2026-04-15 --amount 250000
./bin/ding status customerx
./bin/ding send customerx --dry-run       # render email + Discord embed, no sends
```

A customer that is not yet in the database is bootstrapped from its metadata file
`customers/<id>-ding.yaml` on first use.

## Configuration

Runtime configuration is read entirely from environment variables (no AWS
Secrets Manager). Copy `.env.example` to `.env`; direnv (`.envrc`) loads it.

| Variable | Purpose |
| --- | --- |
| `DING_DB_PATH` | SQLite file path (defaults to `./ding.db` locally) |
| `RESEND_API_KEY` | Resend API key (required for non-dry-run `send`) |
| `DISCORD_PUBLIC_KEY` | Ed25519 key for interaction verification |
| `DISCORD_BOT_TOKEN` | Bot token for slash-command registration |
| `DISCORD_APP_ID` | Application ID for slash-command registration |
| `DISCORD_WEBHOOK_URLS` | JSON `{customer_id: webhook_url}` for monthly posts |
| `DING_DISCORD_ADMIN_USERS` | JSON array of Discord user IDs allowed to `/seed` |
| `DING_LAMBDA_MODE` | `interactions` or `send` (deployment only) |

Per-customer metadata (name, sender, net terms, email subject) lives in
`customers/<id>-ding.yaml`. The database is the source of truth for invoices.

## Usage

| Command | Effect |
| --- | --- |
| `ding migrate` | create/update the schema (idempotent) |
| `ding send <id> [--dry-run]` | render and send the monthly summary (read-only) |
| `ding seed <id> --id INV --issued YYYY-MM-DD --amount CENTS` | insert an invoice |
| `ding mark-paid <id> --id INV --date YYYY-MM-DD [--cents N]` | record a payment |
| `ding status <id>` | print invoices grouped by month |
| `ding validate <id>` | verify the customer exists and dates are valid |

All commands fail closed: a database error, missing customer, or malformed date
exits non-zero and sends nothing.

## Architecture

- **CLI** — local administration and testing against the same SQLite file.
- **Interactions Lambda** (`DING_LAMBDA_MODE=interactions`) — API Gateway →
  Ed25519-verified Discord slash commands (`/mark-paid`, `/status`, `/seed`),
  ephemeral replies.
- **Send Lambda** (`DING_LAMBDA_MODE=send`) — monthly EventBridge cron iterates
  customers, renders the summary, sends via Resend, posts the Discord embed.

Core lateness/outstanding logic is pure and table-tested (`internal/invoice`).
Email templates (`templates/`) are embedded for a self-contained binary.

> ⚠️ **Persistence:** SQLite must live on a persistent, single-writer volume in
> deployment. AWS Lambda `/tmp` is ephemeral and per-instance and is **not** a
> valid system of record. See [docs/references/deployment.md](docs/references/deployment.md).

## Development

```bash
make build   # build bin/ding
make test    # run all tests
make vet     # go vet
make fmt     # go fmt
make lint    # golangci-lint (if installed)
```

## License

See repository.
