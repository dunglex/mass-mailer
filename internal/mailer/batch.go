package mailer

import (
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	mail "github.com/xhit/go-simple-mail/v2"

	ttemplate "text/template"
)

func (app *App) newBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	app.mu.Lock()
	templates := append(append([]EmailTemplate{}, emailTemplates...), app.store.Templates...)
	app.mu.Unlock()
	if err := app.tmpl.ExecuteTemplate(w, "new-batch", PageData{Flash: r.URL.Query().Get("flash"), Templates: templates}); err != nil {
		log.Printf("render new batch: %v", err)
	}
}

func (app *App) createBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		app.redirect(w, r, err)
		return
	}
	receivers, err := receiversFromRequest(r)
	if err != nil {
		app.redirect(w, r, err)
		return
	}
	cc, err := parseCopyRecipients(r.FormValue("cc"))
	if err != nil {
		app.redirect(w, r, fmt.Errorf("cc recipients: %w", err))
		return
	}
	bcc, err := parseCopyRecipients(r.FormValue("bcc"))
	if err != nil {
		app.redirect(w, r, fmt.Errorf("bcc recipients: %w", err))
		return
	}
	batch := Batch{ID: uuid.NewString(), Name: r.FormValue("name"), SenderName: r.FormValue("sender_name"), SenderEmail: r.FormValue("sender_email"), Cc: cc, Bcc: bcc, Subject: r.FormValue("subject"), Body: r.FormValue("body"), Schedule: r.FormValue("schedule"), Receivers: receivers, Attachments: nonEmptyLines(r.FormValue("attachments"))}
	if batch.Name == "" || batch.SenderEmail == "" || batch.Subject == "" || len(batch.Receivers) == 0 {
		app.redirect(w, r, errors.New("name, sender, subject, and at least one receiver are required"))
		return
	}
	if _, _, err := compile(batch); err != nil {
		app.redirect(w, r, err)
		return
	}
	app.mu.Lock()
	app.store.Batches = append(app.store.Batches, batch)
	if name := strings.TrimSpace(r.FormValue("template_name")); name != "" {
		app.store.Templates = upsertTemplate(app.store.Templates, EmailTemplate{Name: name, Subject: batch.Subject, Body: batch.Body})
	}
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
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		data := buildBatchPageData(batch, r.URL.Query().Get("q"), r.URL.Query().Get("status"), page)
		data.Flash = r.URL.Query().Get("flash")
		if err := app.tmpl.ExecuteTemplate(w, "batch-detail", data); err != nil {
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
	case "deliver":
		receivers, kind, _ := deliveryPlan(batch)
		if len(receivers) == 0 {
			err = errors.New("no failed or pending recipients to retry")
		} else {
			err = app.sendReceivers(batch, receivers, kind)
		}
	case "send":
		err = app.sendBatch(batch, "Manual")
	case "retry":
		receivers := retryableReceivers(batch)
		if len(receivers) == 0 {
			err = errors.New("no failed or pending recipients to retry")
		} else {
			err = app.sendReceivers(batch, receivers, "Retry")
		}
	case "test":
		testReceiver := Receiver{"email": r.FormValue("test_email"), "name": "Test receiver"}
		if testReceiver["email"] == "" {
			err = errors.New("test receiver email is required")
		} else {
			var subjectTmpl *ttemplate.Template
			var bodyTmpl *template.Template
			if subjectTmpl, bodyTmpl, err = compile(batch); err == nil {
				testBatch := batch
				testBatch.Cc, testBatch.Bcc = nil, nil
				err = app.sendTo(testBatch, testReceiver, subjectTmpl, bodyTmpl)
			}
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
	if parts[1] == "deliver" || parts[1] == "send" || parts[1] == "retry" || parts[1] == "test" {
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
	return rowsToReceivers(rows)
}

func rowsToReceivers(rows [][]string) ([]Receiver, error) {
	if len(rows) < 2 {
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
				header := strings.TrimSpace(headers[index])
				value = strings.TrimSpace(value)
				if strings.EqualFold(header, "email") {
					header = "email"
					value = normalizeEmail(value)
				}
				receiver[header] = value
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

func normalizeEmail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("mailto:") && strings.EqualFold(value[:len("mailto:")], "mailto:") {
		value = strings.TrimSpace(strings.SplitN(value[len("mailto:"):], "?", 2)[0])
		value = strings.Trim(value, "<>")
	}
	return value
}

func parseCopyRecipients(raw string) ([]string, error) {
	lines := nonEmptyLines(strings.ReplaceAll(raw, "\r", ""))
	if len(lines) == 0 {
		return nil, nil
	}
	addresses, err := netmail.ParseAddressList(strings.Join(lines, ","))
	if err != nil {
		return nil, err
	}
	recipients := make([]string, len(addresses))
	for index, address := range addresses {
		recipients[index] = address.String()
	}
	return recipients, nil
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
	return app.sendReceivers(batch, batch.Receivers, kind)
}

func (app *App) sendReceivers(batch Batch, receivers []Receiver, kind string) error {
	subjectTmpl, bodyTmpl, err := compile(batch)
	if err != nil {
		return fmt.Errorf("compile templates: %w", err)
	}
	var sendErrors []error
	for _, receiver := range receivers {
		if err := app.sendTo(batch, receiver, subjectTmpl, bodyTmpl); err != nil {
			app.recordLog(batch.ID, kind, receiver["email"], err)
			sendErrors = append(sendErrors, fmt.Errorf("send to %s: %w", receiver["email"], err))
			continue
		}
		app.recordLog(batch.ID, kind, receiver["email"], nil)
	}
	app.mu.Lock()
	if len(sendErrors) == 0 {
		for index := range app.store.Batches {
			if app.store.Batches[index].ID == batch.ID {
				app.store.Batches[index].LastSentAt = time.Now()
				break
			}
		}
	}
	err = errors.Join(append(sendErrors, app.save())...)
	app.mu.Unlock()
	return err
}

func upsertTemplate(templates []EmailTemplate, template EmailTemplate) []EmailTemplate {
	for index := range templates {
		if strings.EqualFold(templates[index].Name, template.Name) {
			templates[index] = template
			return templates
		}
	}
	return append(templates, template)
}

func buildBatchPageData(batch Batch, query, status string, page int) BatchPageData {
	const pageSize = 25
	query = strings.TrimSpace(query)
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "pending" && status != "sent" && status != "failed" {
		status = ""
	}
	latest := latestBatchDeliveries(batch)
	var recipients []RecipientView
	for _, receiver := range batch.Receivers {
		view := RecipientView{Receiver: receiver, Status: "Pending"}
		if entry, ok := latest[strings.ToLower(receiver["email"])]; ok {
			view.Status, view.Message = entry.Status, entry.Message
			view.AttemptKind, view.AttemptedAt, view.HasAttempt = entry.Kind, entry.At, true
		}
		if status != "" && !strings.EqualFold(view.Status, status) {
			continue
		}
		if query != "" && !receiverMatches(receiver, query) {
			continue
		}
		recipients = append(recipients, view)
	}
	slices.SortStableFunc(recipients, func(left, right RecipientView) int {
		if left.HasAttempt != right.HasAttempt {
			if !left.HasAttempt {
				return -1
			}
			return 1
		}
		if left.AttemptedAt.After(right.AttemptedAt) {
			return -1
		}
		if left.AttemptedAt.Before(right.AttemptedAt) {
			return 1
		}
		return 0
	})
	totalPages := (len(recipients) + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(recipients))
	_, _, deliveryLabel := deliveryPlan(batch)
	data := BatchPageData{Batch: batch, Recipients: recipients[start:end], Query: query, StatusFilter: status, Page: page, TotalPages: totalPages, TotalMatches: len(recipients), DeliveryLabel: deliveryLabel}
	if page > 1 {
		data.PreviousPage = page - 1
	}
	if page < totalPages {
		data.NextPage = page + 1
	}
	return data
}

func latestRecipientLogs(logs []LogEntry) map[string]LogEntry {
	latest := make(map[string]LogEntry)
	for _, entry := range logs {
		key := strings.ToLower(entry.Recipient)
		if entry.Kind == "Test" || key == "" {
			continue
		}
		if current, ok := latest[key]; !ok || entry.At.After(current.At) {
			latest[key] = entry
		}
	}
	return latest
}

func latestBatchDeliveries(batch Batch) map[string]LogEntry {
	latest := latestRecipientLogs(batch.Logs)
	for recipient, entry := range batch.Deliveries {
		latest[strings.ToLower(recipient)] = entry
	}
	return latest
}

func receiverMatches(receiver Receiver, query string) bool {
	query = strings.ToLower(query)
	for _, value := range receiver {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func retryableReceivers(batch Batch) []Receiver {
	latest := latestBatchDeliveries(batch)
	var receivers []Receiver
	for _, receiver := range batch.Receivers {
		entry, attempted := latest[strings.ToLower(receiver["email"])]
		if !attempted || entry.Status != "Sent" {
			receivers = append(receivers, receiver)
		}
	}
	return receivers
}

func deliveryPlan(batch Batch) ([]Receiver, string, string) {
	receivers := retryableReceivers(batch)
	if len(latestBatchDeliveries(batch)) == 0 {
		return receivers, "Manual", "Send now"
	}
	if len(receivers) > 0 {
		return receivers, "Retry", "Retry unsent"
	}
	return nil, "", ""
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
			if kind != "Test" {
				if app.store.Batches[index].Deliveries == nil {
					app.store.Batches[index].Deliveries = make(map[string]LogEntry)
				}
				app.store.Batches[index].Deliveries[strings.ToLower(recipient)] = entry
			}
			break
		}
	}
	if err := app.save(); err != nil {
		log.Printf("save delivery log: %v", err)
	}
}

func (app *App) sendTo(batch Batch, receiver Receiver, subjectTmpl *ttemplate.Template, bodyTmpl *template.Template) error {
	app.mu.Lock()
	settings := app.store.Settings
	app.mu.Unlock()
	if settings.Host == "" || settings.Port == 0 {
		return errors.New("configure the SMTP server first")
	}
	subject, err := render(subjectTmpl, receiver)
	if err != nil {
		return fmt.Errorf("render subject: %w", err)
	}
	body, err := render(bodyTmpl, receiver)
	if err != nil {
		return fmt.Errorf("render body: %w", err)
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
	message.SetFrom(formatSender(batch.SenderName, batch.SenderEmail))
	message.AddTo(receiver["email"])
	addCopyRecipients(message, batch)
	message.SetSubject(subject)
	message.SetBody(mail.TextHTML, body)
	for _, attachment := range batch.Attachments {
		message.AddAttachment(attachment)
	}
	return message.Send(smtp)
}

func addCopyRecipients(message *mail.Email, batch Batch) {
	if len(batch.Cc) > 0 {
		message.AddCc(batch.Cc...)
	}
	if len(batch.Bcc) > 0 {
		message.AddBcc(batch.Bcc...)
	}
}

func formatSender(name, address string) string {
	return (&netmail.Address{Name: name, Address: address}).String()
}
