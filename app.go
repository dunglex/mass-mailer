package main

import (
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
)

type Receiver map[string]string

type LogEntry struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Recipient string    `json:"recipient"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
}

type Batch struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	SenderName  string     `json:"senderName"`
	SenderEmail string     `json:"senderEmail"`
	Subject     string     `json:"subject"`
	Body        string     `json:"body"`
	Schedule    string     `json:"schedule"`
	Attachments []string   `json:"attachments"`
	Receivers   []Receiver `json:"receivers"`
	LastSentAt  time.Time  `json:"lastSentAt"`
	Logs        []LogEntry `json:"logs"`
}

type Store struct {
	Settings Settings `json:"settings"`
	Batches  []Batch  `json:"batches"`
}

type App struct {
	mu        sync.Mutex
	store     Store
	scheduler gocron.Scheduler
	jobs      map[string]gocron.Job
	tmpl      *template.Template
}
type EmailTemplate struct {
	Name    string
	Subject string
	Body    string
}

type PageData struct {
	Store     Store
	Flash     string
	Templates []EmailTemplate
}

type BatchPageData struct {
	Batch Batch
	Flash string
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (app *App) redirect(w http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(w, r, "/?flash="+template.URLQueryEscaper(resultMessage(err)), http.StatusSeeOther)
}

func resultMessage(err error) string {
	if err != nil {
		return err.Error()
	}
	return "Saved"
}
