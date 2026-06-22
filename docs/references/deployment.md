# ding — deployment runbook

This project ships **no infrastructure-as-code**. AWS resources are created
manually in the console; this document is the runbook. Runtime configuration is
delivered entirely through environment variables (no AWS Secrets Manager).

## 1. Build artifact

The same Go binary serves the CLI and both Lambda functions. Build a Linux
`bootstrap` binary for the `provided.al2`/`provided.al2023` custom runtime:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/ding
zip ding-lambda.zip bootstrap
```

The SQLite driver is pure Go (`glebarez/sqlite`), so `CGO_ENABLED=0` static
builds work and cross-compile cleanly.

## 2. ⚠️ Persistence model (read first)

State is a single SQLite file at `DING_DB_PATH`. SQLite is a single-writer,
local-file database, so the deployed runtime **must** provide persistent,
single-writer storage:

- **Bare Lambda `/tmp` is NOT valid.** It is ephemeral (wiped on cold start) and
  per-execution-environment (not shared between the interactions and send
  functions, nor across concurrent instances). Invoice writes would be lost and
  invisible to the other function.
- **Use a persistent, shared, single-writer volume.** Options:
  - **Amazon EFS** mounted to both Lambda functions (set `DING_DB_PATH` to the
    mount path, e.g. `/mnt/ding/ding.db`). Requires the functions in a VPC with
    the EFS access point attached. Note SQLite-over-NFS locking caveats; keep
    concurrency low.
  - A long-running container (ECS/Fargate/App Runner/EC2) with an attached EBS or
    EFS volume, if you prefer not to use Lambda.

Run `ding migrate` against the same `DING_DB_PATH` once before first use.

## 3. Environment variables

Set these as Lambda environment variables in the console (see `.env.example`):

| Variable | Used by | Notes |
| --- | --- | --- |
| `DING_DB_PATH` | all | Path on the persistent volume, e.g. `/mnt/ding/ding.db` |
| `RESEND_API_KEY` | send | Resend transactional API key |
| `DISCORD_PUBLIC_KEY` | interactions | Ed25519 public key from the Discord app |
| `DISCORD_BOT_TOKEN` | (registration) | Bot token, used out of band to register commands |
| `DISCORD_APP_ID` | (registration) | Discord application ID |
| `DISCORD_WEBHOOK_URLS` | send | JSON `{customer_id: webhook_url}` |
| `DING_DISCORD_ADMIN_USERS` | interactions | JSON array of Discord user IDs for `/seed` |
| `DING_LAMBDA_MODE` | both | `interactions` or `send`; selects the handler |

## 4. AWS console setup

1. **IAM role** for the functions: basic Lambda execution logs, plus VPC/EFS
   access if using EFS.
2. **Interactions function** (`ding-interactions`): runtime `provided.al2023`,
   handler `bootstrap`, env `DING_LAMBDA_MODE=interactions` + the variables
   above. Front it with an **HTTP API (API Gateway)** route `POST /interactions`.
3. **Send function** (`ding-send`): same artifact, env `DING_LAMBDA_MODE=send`.
   Add an **EventBridge** schedule `cron(0 13 1 * ? *)` (1st of month, 13:00 UTC)
   targeting it.
4. Mount the persistent volume (EFS) to both functions and point `DING_DB_PATH`
   at it.

## 5. Discord slash-command registration (one-time)

Register the commands against the application (the `customer` option scopes each
command to a customer, since state is multi-customer):

```bash
curl -X POST "https://discord.com/api/v10/applications/${DISCORD_APP_ID}/commands" \
  -H "Authorization: Bot ${DISCORD_BOT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"name":"mark-paid","description":"Record a payment","options":[
        {"name":"customer","description":"Customer id","type":3,"required":true},
        {"name":"invoice-id","description":"Invoice id","type":3,"required":true},
        {"name":"date","description":"YYYY-MM-DD","type":3,"required":false},
        {"name":"cents","description":"Amount paid in cents","type":4,"required":false}]}'
```

Repeat for `status` (option: `customer`) and `seed` (options: `customer`, `id`,
`issued`, `amount`). Set the application's **Interactions Endpoint URL** to the
API Gateway `POST /interactions` URL; Discord sends a PING that the handler
answers with PONG.

## 6. Verify

- `GET`/`POST` a Discord PING to the endpoint → `{"type":1}` (PONG).
- Invoke `ding-send` manually → email via Resend + Discord embed posted.
- Locally, against the deployed volume: `ding status <customer>`.
