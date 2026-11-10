package mailer

import (
	"errors"
	"fmt"
	"html/template"
	netmail "net/mail"
	ttemplate "text/template"

	mail "github.com/xhit/go-simple-mail/v2"
)

func (app *App) sendPendingReceivers(batch Batch, receivers []Receiver, kind string) error {
	if len(receivers) == 0 {
		return errors.New("no failed or pending recipients to retry")
	}
	return app.sendReceivers(batch, receivers, kind)
}

func (app *App) sendTestBatch(batch Batch, email string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("test receiver email is required")
	}
	receiver := Receiver{"email": email, "name": "Test receiver"}
	subjectTmpl, bodyTmpl, err := compile(batch)
	if err == nil {
		// A test message must never reach the batch's copy recipients.
		batch.Cc, batch.Bcc = nil, nil
		err = app.sendTo(batch, receiver, subjectTmpl, bodyTmpl)
	}
	app.recordLog(batch.ID, "Test", email, err)
	return err
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
	return buildMessage(batch, receiver, subject, body).Send(smtp)
}

func buildMessage(batch Batch, receiver Receiver, subject, body string) *mail.Email {
	message := mail.NewMSG()
	message.SetFrom(formatSender(batch.SenderName, batch.SenderEmail))
	message.AddTo(normalizeEmail(receiver["email"]))
	addCopyRecipients(message, batch)
	message.SetSubject(subject)
	message.SetBody(mail.TextHTML, body)
	for _, attachment := range batch.Attachments {
		message.AddAttachment(attachment)
	}
	return message
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
	return (&netmail.Address{Name: name, Address: normalizeEmail(address)}).String()
}
