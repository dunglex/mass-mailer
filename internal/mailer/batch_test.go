package mailer

import (
	"bytes"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	mail "github.com/xhit/go-simple-mail/v2"
)

func TestParseCopyRecipients(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty"},
		{name: "comma separated", raw: "first@example.com, second@example.com", want: []string{"<first@example.com>", "<second@example.com>"}},
		{name: "newline separated", raw: "first@example.com\nSecond Person <second@example.com>", want: []string{"<first@example.com>", `"Second Person" <second@example.com>`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCopyRecipients(test.raw)
			if err != nil {
				t.Fatalf("parseCopyRecipients: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("recipients = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseCopyRecipientsRejectsInvalidAddress(t *testing.T) {
	if _, err := parseCopyRecipients("not-an-email"); err == nil {
		t.Fatal("parseCopyRecipients accepted an invalid address")
	}
}

func TestAddCopyRecipientsKeepsBccOutOfMessageHeaders(t *testing.T) {
	message := mail.NewMSG()
	message.SetFrom("sender@example.com")
	message.AddTo("primary@example.com")
	message.SetSubject("Subject")
	message.SetBody(mail.TextPlain, "Body")
	addCopyRecipients(message, Batch{
		Cc:  []string{"<copy@example.com>"},
		Bcc: []string{"<hidden@example.com>"},
	})

	recipients := message.GetRecipients()
	for _, expected := range []string{"primary@example.com", "copy@example.com", "hidden@example.com"} {
		if !slices.Contains(recipients, expected) {
			t.Errorf("envelope recipients %q do not contain %q", recipients, expected)
		}
	}
	serialized := message.GetMessage()
	if !strings.Contains(serialized, "Cc:") || !strings.Contains(serialized, "copy@example.com") {
		t.Errorf("message does not contain Cc header: %s", serialized)
	}
	if strings.Contains(serialized, "hidden@example.com") {
		t.Errorf("message exposes Bcc recipient: %s", serialized)
	}
}

func TestCreateBatchPersistsCopyRecipients(t *testing.T) {
	withDataFile(t, "")
	form := url.Values{
		"name":         {"Batch"},
		"sender_email": {"sender@example.com"},
		"subject":      {"Subject"},
		"body":         {"<p>Body</p>"},
		"receivers":    {"name,email\nAda,ada@example.com"},
		"cc":           {"copy@example.com, Second Person <second@example.com>"},
		"bcc":          {"hidden@example.com"},
	}
	request := httptest.NewRequest(http.MethodPost, "/batches", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	app := &App{}

	app.createBatch(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if len(app.store.Batches) != 1 {
		t.Fatalf("batches = %#v", app.store.Batches)
	}
	batch := app.store.Batches[0]
	wantCc := []string{"<copy@example.com>", `"Second Person" <second@example.com>`}
	if !reflect.DeepEqual(batch.Cc, wantCc) {
		t.Errorf("Cc = %#v, want %#v", batch.Cc, wantCc)
	}
	if !reflect.DeepEqual(batch.Bcc, []string{"<hidden@example.com>"}) {
		t.Errorf("Bcc = %#v", batch.Bcc)
	}
}

func TestCreateBatchRejectsOversizedMultipartRequest(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	field, err := writer.CreateFormField("receivers")
	if err != nil {
		t.Fatalf("create receivers field: %v", err)
	}
	if _, err := field.Write(bytes.Repeat([]byte("x"), maxBatchRequestSize)); err != nil {
		t.Fatalf("write receivers field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/batches", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	app := &App{}

	app.createBatch(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if len(app.store.Batches) != 0 {
		t.Fatalf("oversized request persisted batches: %#v", app.store.Batches)
	}
	if location := response.Header().Get("Location"); !strings.Contains(location, "request+body+too+large") {
		t.Fatalf("Location = %q, want request body too large error", location)
	}
}

func TestBuildBatchPageDataFiltersAndUsesLatestStatus(t *testing.T) {
	now := time.Now()
	batch := Batch{
		Receivers: []Receiver{{"name": "Ada", "email": "ada@example.com"}, {"name": "Grace", "email": "grace@example.com"}},
		Logs: []LogEntry{
			{At: now, Kind: "Retry", Recipient: "ada@example.com", Status: "Sent"},
			{At: now.Add(-time.Minute), Kind: "Manual", Recipient: "ada@example.com", Status: "Failed", Message: "old failure"},
			{At: now.Add(time.Minute), Kind: "Test", Recipient: "grace@example.com", Status: "Sent"},
		},
	}

	data := buildBatchPageData(batch, "ada", "sent", 1)
	if data.TotalMatches != 1 || len(data.Recipients) != 1 {
		t.Fatalf("recipients = %#v, total = %d; want only Ada", data.Recipients, data.TotalMatches)
	}
	if data.Recipients[0].Status != "Sent" || data.Recipients[0].Message != "" {
		t.Fatalf("latest recipient status = %#v, want Sent", data.Recipients[0])
	}
	if !data.Recipients[0].HasAttempt || data.Recipients[0].AttemptKind != "Retry" || !data.Recipients[0].AttemptedAt.Equal(now) {
		t.Fatalf("latest attempt = %#v", data.Recipients[0])
	}

	pending := buildBatchPageData(batch, "", "pending", 1)
	if pending.TotalMatches != 1 || pending.Recipients[0].Receiver["email"] != "grace@example.com" {
		t.Fatalf("pending recipients = %#v, want Grace", pending.Recipients)
	}
}

func TestBuildBatchPageDataPaginates(t *testing.T) {
	batch := Batch{}
	for index := 0; index < 30; index++ {
		batch.Receivers = append(batch.Receivers, Receiver{"email": string(rune('a'+index)) + "@example.com"})
	}
	data := buildBatchPageData(batch, "", "", 2)
	if len(data.Recipients) != 5 || data.TotalPages != 2 || data.PreviousPage != 1 || data.NextPage != 0 {
		t.Fatalf("page data = %#v", data)
	}
}

func TestBuildBatchPageDataSortsNeverAttemptedThenLatest(t *testing.T) {
	older := time.Date(2026, time.September, 24, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	batch := Batch{
		Receivers: []Receiver{
			{"email": "older@example.com"},
			{"email": "pending-first@example.com"},
			{"email": "newer@example.com"},
			{"email": "pending-second@example.com"},
		},
		Logs: []LogEntry{
			{At: older, Recipient: "older@example.com", Status: "Sent"},
			{At: newer, Recipient: "newer@example.com", Status: "Failed"},
		},
	}

	data := buildBatchPageData(batch, "", "", 1)
	want := []string{"pending-first@example.com", "pending-second@example.com", "newer@example.com", "older@example.com"}
	for index, recipient := range data.Recipients {
		if got := recipient.Receiver["email"]; got != want[index] {
			t.Fatalf("recipient %d = %q, want %q", index, got, want[index])
		}
	}
}

func TestRetryableReceiversExcludesSentRecipients(t *testing.T) {
	batch := Batch{
		Receivers: []Receiver{{"email": "sent@example.com"}, {"email": "failed@example.com"}, {"email": "pending@example.com"}},
		Logs: []LogEntry{
			{Kind: "Manual", Recipient: "sent@example.com", Status: "Sent"},
			{Kind: "Manual", Recipient: "failed@example.com", Status: "Failed"},
		},
	}
	got := retryableReceivers(batch)
	if len(got) != 2 || got[0]["email"] != "failed@example.com" || got[1]["email"] != "pending@example.com" {
		t.Fatalf("retryable receivers = %#v", got)
	}
}

func TestDeliveryPlanUsesOneStateAwareAction(t *testing.T) {
	receivers := []Receiver{{"email": "first@example.com"}, {"email": "second@example.com"}}
	tests := []struct {
		name      string
		batch     Batch
		wantKind  string
		wantLabel string
		wantCount int
	}{
		{name: "fresh batch", batch: Batch{Receivers: receivers}, wantKind: "Manual", wantLabel: "Send now", wantCount: 2},
		{name: "partial batch", batch: Batch{Receivers: receivers, Logs: []LogEntry{{Recipient: "first@example.com", Status: "Sent"}}}, wantKind: "Retry", wantLabel: "Retry unsent", wantCount: 1},
		{name: "completed batch", batch: Batch{Receivers: receivers, Logs: []LogEntry{{Recipient: "first@example.com", Status: "Sent"}, {Recipient: "second@example.com", Status: "Sent"}}}, wantCount: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, kind, label := deliveryPlan(test.batch)
			if len(got) != test.wantCount || kind != test.wantKind || label != test.wantLabel {
				t.Fatalf("deliveryPlan = (%d, %q, %q), want (%d, %q, %q)", len(got), kind, label, test.wantCount, test.wantKind, test.wantLabel)
			}
		})
	}
}

func TestBatchDetailCombinesDeliveryControlsAndRecipientLog(t *testing.T) {
	attemptedAt := time.Date(2026, time.September, 25, 10, 30, 0, 0, time.UTC)
	batch := Batch{
		ID:        "batch-1",
		Name:      "Batch",
		Receivers: []Receiver{{"name": "Ada", "email": "ada@example.com"}, {"name": "Grace", "email": "grace@example.com"}},
		Logs:      []LogEntry{{At: attemptedAt, Kind: "Manual", Recipient: "ada@example.com", Status: "Sent"}},
	}
	tmpl := template.Must(template.New("base").Funcs(template.FuncMap{"receiverName": func(receiver Receiver) string {
		if receiver["name"] != "" {
			return receiver["name"]
		}
		return receiver["email"]
	}}).ParseFS(templateFiles, "templates/*.gohtml"))
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "batch-detail", buildBatchPageData(batch, "", "", 1)); err != nil {
		t.Fatalf("render batch detail: %v", err)
	}
	html := output.String()
	for _, expected := range []string{`action="/batches/batch-1/deliver"`, "Retry unsent", "Last attempt", "2026-09-25 10:30:00", "Manual"} {
		if !strings.Contains(html, expected) {
			t.Errorf("batch detail does not contain %q", expected)
		}
	}
	for _, removed := range []string{`action="/batches/batch-1/send"`, `action="/batches/batch-1/retry"`, "Delivery log"} {
		if strings.Contains(html, removed) {
			t.Errorf("batch detail still contains %q", removed)
		}
	}
}

func TestBatchDetailHidesDeliveryActionWhenCompleted(t *testing.T) {
	batch := Batch{
		ID:        "batch-1",
		Receivers: []Receiver{{"email": "ada@example.com"}},
		Logs:      []LogEntry{{Recipient: "ada@example.com", Status: "Sent"}},
	}
	tmpl := template.Must(template.New("base").Funcs(template.FuncMap{"receiverName": func(receiver Receiver) string { return receiver["email"] }}).ParseFS(templateFiles, "templates/*.gohtml"))
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "batch-detail", buildBatchPageData(batch, "", "", 1)); err != nil {
		t.Fatalf("render batch detail: %v", err)
	}
	if strings.Contains(output.String(), `/batches/batch-1/deliver`) {
		t.Fatal("completed batch still renders a delivery action")
	}
}

func TestBuildBatchPageDataUsesPersistedDeliveryStatus(t *testing.T) {
	batch := Batch{
		Receivers:  []Receiver{{"email": "ada@example.com"}},
		Deliveries: map[string]LogEntry{"ada@example.com": {Status: "Sent"}},
	}
	data := buildBatchPageData(batch, "", "", 1)
	if data.Recipients[0].Status != "Sent" {
		t.Fatalf("status = %q, want Sent", data.Recipients[0].Status)
	}
}

func TestFormatSenderQuotesSpecialCharacters(t *testing.T) {
	got := formatSender("Team [Europe]", "hello@example.com")
	if got != `"Team [Europe]" <hello@example.com>` {
		t.Fatalf("formatSender() = %q", got)
	}
}

func TestUpsertTemplateReplacesNameCaseInsensitively(t *testing.T) {
	templates := []EmailTemplate{{Name: "Newsletter", Subject: "Old", Body: "Old body"}}
	got := upsertTemplate(templates, EmailTemplate{Name: "newsletter", Subject: "New", Body: "New body"})
	if len(got) != 1 || got[0].Subject != "New" || got[0].Body != "New body" {
		t.Fatalf("templates = %#v", got)
	}
}
