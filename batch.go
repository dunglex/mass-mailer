package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	mail "github.com/xhit/go-simple-mail/v2"
)

func (app *App) newBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if err := app.tmpl.ExecuteTemplate(w, "new-batch", PageData{Flash: r.URL.Query().Get("flash"), Templates: emailTemplates}); err != nil {
		log.Printf("render new batch: %v", err)
	}
}

func (app *App) createBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	receivers, err := parseReceivers(r.FormValue("receivers"))
	if err != nil {
		app.redirect(w, r, err)
		return
	}
	batch := Batch{ID: uuid.NewString(), Name: r.FormValue("name"), SenderName: r.FormValue("sender_name"), SenderEmail: r.FormValue("sender_email"), Subject: r.FormValue("subject"), Body: r.FormValue("body"), Schedule: r.FormValue("schedule"), Receivers: receivers, Attachments: nonEmptyLines(r.FormValue("attachments"))}
	if batch.Name == "" || batch.SenderEmail == "" || batch.Subject == "" || len(batch.Receivers) == 0 {
		app.redirect(w, r, errors.New("name, sender, subject, and at least one receiver are required"))
		return
	}
	app.mu.Lock()
	app.store.Batches = append(app.store.Batches, batch)
	err = app.save()
	if err == nil {
		err = app.schedule(batch)
	}
	app.mu.Unlock()
	app.redirect(w, r, err)
}

func (app *App) batchAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/batches/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		batch, found := app.findBatch(parts[0])
		if !found {
			http.NotFound(w, r)
			return
		}
		if err := app.tmpl.ExecuteTemplate(w, "batch-detail", BatchPageData{Batch: batch, Flash: r.URL.Query().Get("flash")}); err != nil {
			log.Printf("render batch detail: %v", err)
		}
		return
	}
	if r.Method != http.MethodPost || len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	batch, found := app.findBatch(parts[0])
	if !found {
		app.redirect(w, r, errors.New("batch not found"))
		return
	}
	var err error
	switch parts[1] {
	case "send":
		err = app.sendBatch(batch, "Manual")
	case "test":
		testReceiver := Receiver{"email": r.FormValue("test_email"), "name": "Test receiver"}
		if testReceiver["email"] == "" {
			err = errors.New("test receiver email is required")
		} else {
			err = app.sendTo(batch, testReceiver)
			app.recordLog(batch.ID, "Test", testReceiver["email"], err)
		}
	case "delete":
		app.mu.Lock()
		if job, ok := app.jobs[batch.ID]; ok {
			_ = app.scheduler.RemoveJob(job.ID())
			delete(app.jobs, batch.ID)
		}
		for index := range app.store.Batches {
			if app.store.Batches[index].ID == batch.ID {
				app.store.Batches = append(app.store.Batches[:index], app.store.Batches[index+1:]...)
				break
			}
		}
		err = app.save()
		app.mu.Unlock()
	default:
		http.NotFound(w, r)
		return
	}
	if parts[1] == "send" || parts[1] == "test" {
		http.Redirect(w, r, "/batches/"+batch.ID+"?flash="+template.URLQueryEscaper(resultMessage(err)), http.StatusSeeOther)
		return
	}
	app.redirect(w, r, err)
}

func (app *App) findBatch(id string) (Batch, bool) {
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, batch := range app.store.Batches {
		if batch.ID == id {
			return batch, true
		}
	}
	return Batch{}, false
}

func parseReceivers(raw string) ([]Receiver, error) {
	rows, err := csv.NewReader(strings.NewReader(raw)).ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, errors.New("receivers must be CSV with a header and at least one row")
	}
	headers := rows[0]
	foundEmail := false
	for _, header := range headers {
		if strings.EqualFold(strings.TrimSpace(header), "email") {
			foundEmail = true
		}
	}
	if !foundEmail {
		return nil, errors.New("receiver CSV needs an email column")
	}
	var receivers []Receiver
	for _, row := range rows[1:] {
		receiver := Receiver{}
		for index, value := range row {
			if index < len(headers) {
				receiver[strings.TrimSpace(headers[index])] = strings.TrimSpace(value)
			}
		}
		if receiver["email"] != "" {
			receivers = append(receivers, receiver)
		}
	}
	if len(receivers) == 0 {
		return nil, errors.New("no valid receiver emails found")
	}
	return receivers, nil
}

func (app *App) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.tmpl.ExecuteTemplate(w, "index", PageData{Store: app.store, Flash: r.URL.Query().Get("flash")}); err != nil {
		log.Printf("render dashboard: %v", err)
	}
}

func (app *App) schedule(batch Batch) error {
	if strings.TrimSpace(batch.Schedule) == "" {
		return nil
	}
	job, err := app.scheduler.NewJob(gocron.CronJob(batch.Schedule, false), gocron.NewTask(func() {
		if err := app.sendBatch(batch, "Scheduled"); err != nil {
			log.Printf("scheduled batch %q: %v", batch.Name, err)
		}
	}))
	if err == nil {
		app.jobs[batch.ID] = job
	}
	return err
}
func (app *App) sendBatch(batch Batch, kind string) error {
	for _, receiver := range batch.Receivers {
		if err := app.sendTo(batch, receiver); err != nil {
			app.recordLog(batch.ID, kind, receiver["email"], err)
			return fmt.Errorf("send to %s: %w", receiver["email"], err)
		}
		app.recordLog(batch.ID, kind, receiver["email"], nil)
	}
	app.mu.Lock()
	for index := range app.store.Batches {
		if app.store.Batches[index].ID == batch.ID {
			app.store.Batches[index].LastSentAt = time.Now()
			break
		}
	}
	err := app.save()
	app.mu.Unlock()
	return err
}

func (app *App) recordLog(batchID, kind, recipient string, sendErr error) {
	entry := LogEntry{At: time.Now(), Kind: kind, Recipient: recipient, Status: "Sent"}
	if sendErr != nil {
		entry.Status, entry.Message = "Failed", sendErr.Error()
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for index := range app.store.Batches {
		if app.store.Batches[index].ID == batchID {
			app.store.Batches[index].Logs = append([]LogEntry{entry}, app.store.Batches[index].Logs...)
			if len(app.store.Batches[index].Logs) > 200 {
				app.store.Batches[index].Logs = app.store.Batches[index].Logs[:200]
			}
			break
		}
	}
	if err := app.save(); err != nil {
		log.Printf("save delivery log: %v", err)
	}
}

func (app *App) sendTo(batch Batch, receiver Receiver) error {
	app.mu.Lock()
	settings := app.store.Settings
	app.mu.Unlock()
	if settings.Host == "" || settings.Port == 0 {
		return errors.New("configure the SMTP server first")
	}
	client := mail.NewSMTPClient()
	client.Host, client.Port, client.Username, client.Password = settings.Host, settings.Port, settings.Username, settings.Password
	switch settings.Encryption {
	case "ssl":
		client.Encryption = mail.EncryptionSSLTLS
	case "tls":
		client.Encryption = mail.EncryptionSTARTTLS
	default:
		client.Encryption = mail.EncryptionNone
	}
	smtp, err := client.Connect()
	if err != nil {
		return err
	}
	message := mail.NewMSG()
	message.SetFrom(fmt.Sprintf("%s <%s>", batch.SenderName, batch.SenderEmail))
	message.AddTo(receiver["email"])
	message.SetSubject(renderText(batch.Subject, receiver))
	message.SetBody(mail.TextHTML, renderText(batch.Body, receiver))
	for _, attachment := range batch.Attachments {
		message.AddAttachment(attachment)
	}
	return message.Send(smtp)
}

func renderText(source string, receiver Receiver) string {
	for key, value := range receiver {
		source = strings.ReplaceAll(source, "{{"+key+"}}", html.EscapeString(value))
	}
	return source
}
