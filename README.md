# Mass Mailer

A small server-rendered Go application for creating recipient batches, sending test messages, and scheduling SMTP delivery.

## Project layout

- `cmd/mass-mailer` contains the application executable.
- `cmd/entrypoint` contains the container entrypoint executable.
- `internal/mailer` contains application logic, tests, and embedded web templates.
- `scripts` contains operational shell helpers.

## Run

```powershell
go run ./cmd/mass-mailer
```

Open `http://localhost:8080`. Set `PORT` to use a different port:

```powershell
$env:PORT = "8081"
go run ./cmd/mass-mailer
```

Configure the SMTP server from the web interface before sending. Application data, including the SMTP password, is stored locally in `data/mass-mailer.json`; protect this file and do not commit it. Set `DATA_DIR` to store it elsewhere.

## Authentication

The web interface is protected with HTTP Basic Auth when `AUTH_USER` and `AUTH_PASSWORD` are set:

```bash
AUTH_USER=admin AUTH_PASSWORD=change-me go run ./cmd/mass-mailer
```

If both are unset the application serves **with no authentication** and logs a warning at startup. Because anyone reaching the port gets a mail-sending console and the stored SMTP credentials, only run it unauthenticated on a trusted local machine.

## Batches

Receivers can be provided either by uploading an `.xlsx` or `.csv` file, or by pasting CSV into the receivers textarea. An uploaded file takes precedence over pasted CSV when both are present. Either way, the header row (the first row of the CSV, or the first sheet of the workbook) defines fields that can be included in the subject or HTML body:

```csv
name,email,phone
Ada Lovelace,ada@example.com,+15550100
```

Subjects and bodies are real Go templates (`text/template` for the subject, `html/template` for the body), so each CSV header is available as a field: use `{{.name}}`, `{{.email}}`, `{{.phone}}`, or any other CSV header in the subject and email HTML. Bare placeholders like `{{name}}` are still accepted for backward compatibility, but new templates should prefer the `{{.name}}` form.

Conditionals and loops now work, for example `{{if .vip}}Thanks for being a VIP!{{end}}`. CSV headers containing spaces are not valid Go identifiers, so access them with `{{index . "first name"}}` instead of `{{."first name"}}`. A recipient missing a given column renders as empty text rather than an error.

Attachments are local file paths, one per line.

Optional Cc and Bcc lists accept comma- or newline-separated addresses, including display names such as `Team Lead <lead@example.com>`. Each Cc/Bcc address receives a copy of every personalized batch delivery; Bcc addresses are omitted from message headers. Test sends go only to the test address.

Schedules use five-field cron expressions such as `0 9 * * 1-5` for 09:00 on weekdays. Leave the schedule blank to send manually.

## Docker

A `Dockerfile` and `compose.yaml` are included for containerized deployment. Copy the sample environment file first, then build and run:

```bash
cp .env.example .env    # set AUTH_USER / AUTH_PASSWORD
make up                 # or: docker compose up --build
```

`make up` (and `make up-dev`, which adds Mailpit) creates `data/` and `attachments/` as your user before starting Compose. Use them in preference to calling `docker compose` directly — see the permissions note below.

This starts the application on port 8080 and persists data in the local `data/` directory.

The runtime uses the public `gcr.io/distroless/static-debian12` image. The Dockerfile explicitly selects root to preserve directory preparation and bind-mount behavior.

### Troubleshooting: `open /data/mass-mailer.json: permission denied`

This appears as a flash message in the web interface when the container cannot write to the mounted data directory. The usual cause is a host filesystem or security policy that prevents container writes. Check with `ls -ldn data`.

Repair an existing directory or restore ownership for local tools with:

```bash
make fix-perms
```

which chowns `data/` and `attachments/` to your user from a root container, so no host `sudo` is needed. `data/.gitkeep` is tracked precisely so the directory exists on a fresh clone and Docker never has to create it.

### Testing with Mailpit

The `dev` profile adds [Mailpit](https://mailpit.axllent.org/), a local SMTP sink that captures every message instead of delivering it:

```bash
cp .env.example .env    # ships pre-pointed at Mailpit
docker compose --profile dev up --build
```

`.env.example` sets `SMTP_HOST=mailpit`, `SMTP_PORT=1025` and `SMTP_ENCRYPTION=none`, so the app comes up already configured — create a batch and hit **Send test** without visiting the Server settings page. Captured mail, including the rendered HTML and any attachments, is at `http://localhost:8025`. Mailpit keeps it across restarts in a named volume; run `docker compose --profile dev down -v` to discard it.

### Seeding SMTP settings from the environment

`SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` and `SMTP_ENCRYPTION` seed the Server settings page **only when no settings have been saved yet**:

- Anything saved from the web interface is written to `data/mass-mailer.json` and wins on every later boot.
- Seeded values are never written to disk, so deleting the data file returns to the environment's defaults.
- `SMTP_ENCRYPTION` accepts `tls` (STARTTLS), `ssl` (implicit TLS) or `none`, and defaults to `tls` when `SMTP_HOST` is set without it. Mailpit listens in plaintext and needs `none`.

This works outside Docker too:

```bash
SMTP_HOST=localhost SMTP_PORT=1025 SMTP_ENCRYPTION=none go run ./cmd/mass-mailer
```

### Batch Attachments

Attachments in batch files are interpreted as server-side file paths. When running in Docker, attachment paths must be relative to the mounted `/attachments` directory (mapped from `./attachments/` on the host). For example, to include an attachment, place the file in `./attachments/` locally and reference it in the batch as `/attachments/filename.pdf`.