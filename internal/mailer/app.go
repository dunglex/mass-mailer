package mailer

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
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	SenderName  string              `json:"senderName"`
	SenderEmail string              `json:"senderEmail"`
	Cc          []string            `json:"cc,omitempty"`
	Bcc         []string            `json:"bcc,omitempty"`
	Subject     string              `json:"subject"`
	Body        string              `json:"body"`
	Schedule    string              `json:"schedule"`
	Attachments []string            `json:"attachments"`
	Receivers   []Receiver          `json:"receivers"`
	LastSentAt  time.Time           `json:"lastSentAt"`
	Logs        []LogEntry          `json:"logs"`
	Deliveries  map[string]LogEntry `json:"deliveries,omitempty"`
}

type Store struct {
	Settings  Settings        `json:"settings"`
	Batches   []Batch         `json:"batches"`
	Templates []EmailTemplate `json:"templates,omitempty"`
}

type App struct {
	mu        sync.Mutex
	store     Store
	scheduler gocron.Scheduler
	jobs      map[string]gocron.Job
	tmpl      *template.Template
}
type EmailTemplate struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type PageData struct {
	Store     Store
	Flash     string
	Templates []EmailTemplate
}

type BatchPageData struct {
	Batch         Batch
	Flash         string
	Recipients    []RecipientView
	Query         string
	StatusFilter  string
	Page          int
	PreviousPage  int
	NextPage      int
	TotalPages    int
	TotalMatches  int
	DeliveryLabel string
}

type RecipientView struct {
	Receiver    Receiver
	Status      string
	Message     string
	AttemptKind string
	AttemptedAt time.Time
	HasAttempt  bool
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
