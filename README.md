# Mass Mailer

A small server-rendered Go application for creating recipient batches, sending test messages, and scheduling SMTP delivery.

## Run

```powershell
go run .
```

Open `http://localhost:8080`. Set `PORT` to use a different port:

```powershell
$env:PORT = "8081"
go run .
```

Configure the SMTP server from the web interface before sending. Application data, including the SMTP password, is stored locally in `data/mass-mailer.json`; protect this file and do not commit it.

## Batches

Receivers are entered as CSV. The header row defines fields that can be included in the subject or HTML body:

```csv
name,email,phone
Ada Lovelace,ada@example.com,+15550100
```

Use `{{name}}`, `{{email}}`, `{{phone}}`, or any other CSV header in the subject and email HTML. Attachments are local file paths, one per line.

Schedules use five-field cron expressions such as `0 9 * * 1-5` for 09:00 on weekdays. Leave the schedule blank to send manually.