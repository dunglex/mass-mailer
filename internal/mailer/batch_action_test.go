package mailer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestBatchActionsPreserveRedirectsAndLogs(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		email     string
		completed bool
		wantFlash string
		wantKind  string
		wantLogs  int
		deleted   bool
	}{
		{name: "deliver fresh", action: "deliver", wantFlash: "send to ada@example.com: configure the SMTP server first", wantKind: "Manual", wantLogs: 1},
		{name: "manual send", action: "send", wantFlash: "send to ada@example.com: configure the SMTP server first", wantKind: "Manual", wantLogs: 1},
		{name: "retry", action: "retry", wantFlash: "send to ada@example.com: configure the SMTP server first", wantKind: "Retry", wantLogs: 1},
		{name: "completed delivery", action: "deliver", completed: true, wantFlash: "no failed or pending recipients to retry"},
		{name: "completed retry", action: "retry", completed: true, wantFlash: "no failed or pending recipients to retry"},
		{name: "missing test email", action: "test", wantFlash: "test receiver email is required"},
		{name: "test delivery", action: "test", email: "test@example.com", wantFlash: "configure the SMTP server first", wantKind: "Test", wantLogs: 1},
		{name: "mailto test delivery", action: "test", email: " MAILTO:test@example.com?subject=Hello ", wantFlash: "configure the SMTP server first", wantKind: "Test", wantLogs: 1},
		{name: "empty mailto test email", action: "test", email: "mailto:", wantFlash: "test receiver email is required"},
		{name: "delete", action: "delete", wantFlash: "Saved", deleted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withDataFile(t, "")
			batch := Batch{ID: "batch-1", Subject: "Hello", Body: "Body", Receivers: []Receiver{{"email": "ada@example.com"}}}
			if test.completed {
				batch.Deliveries = map[string]LogEntry{"ada@example.com": {Status: "Sent"}}
			}
			app := &App{store: Store{Batches: []Batch{batch, {ID: "keep"}}}}
			form := url.Values{"test_email": {test.email}}
			request := httptest.NewRequest(http.MethodPost, "/batches/batch-1/"+test.action, strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()

			app.batchAction(response, request)

			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
			}
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			wantPath := "/batches/batch-1"
			if test.deleted {
				wantPath = "/"
			}
			if location.Path != wantPath || location.Query().Get("flash") != test.wantFlash {
				t.Errorf("redirect = %s, want path %q and flash %q", location, wantPath, test.wantFlash)
			}
			if test.deleted {
				if len(app.store.Batches) != 1 || app.store.Batches[0].ID != "keep" {
					t.Fatalf("remaining batches = %#v", app.store.Batches)
				}
			} else {
				batch = app.store.Batches[0]
				if len(batch.Logs) != test.wantLogs {
					t.Fatalf("logs = %#v, want %d entries", batch.Logs, test.wantLogs)
				}
				if test.wantLogs > 0 {
					entry := batch.Logs[0]
					if entry.Kind != test.wantKind || entry.Status != "Failed" {
						t.Errorf("log = %#v, want kind %q and Failed", entry, test.wantKind)
					}
					if test.wantKind == "Test" && len(batch.Deliveries) != 0 {
						t.Errorf("test send changed delivery state: %#v", batch.Deliveries)
					}
					if test.wantKind == "Test" && entry.Recipient != "test@example.com" {
						t.Errorf("test log recipient = %q, want test@example.com", entry.Recipient)
					}
					if test.wantKind != "Test" && batch.Deliveries["ada@example.com"].Status != "Failed" {
						t.Errorf("delivery failure was not tracked: %#v", batch.Deliveries)
					}
				}
			}
			if test.deleted || test.wantLogs > 0 {
				contents, err := os.ReadFile(dataFile)
				if err != nil {
					t.Fatalf("read saved store: %v", err)
				}
				// Compare persisted representations: JSON omits time.Time's
				// monotonic clock data, so struct equality is not appropriate.
				want, err := json.MarshalIndent(app.store, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(contents, want) {
					t.Error("saved store differs from memory")
				}
			}
		})
	}
}

func TestBatchActionRejectsInvalidRoutes(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/batches/batch-1/unknown"},
		{http.MethodGet, "/batches/batch-1/send"},
		{http.MethodPost, "/batches/batch-1"},
		{http.MethodPost, "/batches/batch-1/send/extra"},
		{http.MethodGet, "/batches/missing"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			app := &App{store: Store{Batches: []Batch{{ID: "batch-1"}}}}
			response := httptest.NewRecorder()
			app.batchAction(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
		})
	}
}
