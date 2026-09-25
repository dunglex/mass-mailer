package mailer

import (
	"strings"
	"testing"
)

func TestRenderCompileAndExecute(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		body        string
		receiver    Receiver
		wantSubject string
		wantBody    string
	}{
		{
			name:        "bare placeholder shim still renders",
			subject:     "Hello {{name}}",
			body:        "<p>Hi {{name}}</p>",
			receiver:    Receiver{"name": "Ada"},
			wantSubject: "Hello Ada",
			wantBody:    "<p>Hi Ada</p>",
		},
		{
			name:        "dotted placeholder renders",
			subject:     "Hello {{.name}}",
			body:        "<p>Hi {{.name}}</p>",
			receiver:    Receiver{"name": "Ada"},
			wantSubject: "Hello Ada",
			wantBody:    "<p>Hi Ada</p>",
		},
		{
			name:        "keywords are not rewritten by the shim",
			subject:     "Hi {{.name}}",
			body:        "{{if .vip}}VIP{{end}}",
			receiver:    Receiver{"name": "Ada", "vip": "yes"},
			wantSubject: "Hi Ada",
			wantBody:    "VIP",
		},
		{
			name:        "if with false condition renders nothing",
			subject:     "Hi {{.name}}",
			body:        "{{if .vip}}VIP{{end}}",
			receiver:    Receiver{"name": "Ada"},
			wantSubject: "Hi Ada",
			wantBody:    "",
		},
		{
			name:        "range works",
			subject:     "Hi {{.name}}",
			body:        "{{range $k, $v := .}}{{$k}}={{$v}};{{end}}",
			receiver:    Receiver{"name": "Ada"},
			wantSubject: "Hi Ada",
			wantBody:    "name=Ada;",
		},
		{
			name:        "missing key renders empty not no value",
			subject:     "Hi {{.missing}}!",
			body:        "<p>{{.missing}}</p>",
			receiver:    Receiver{"name": "Ada"},
			wantSubject: "Hi !",
			wantBody:    "<p></p>",
		},
		{
			name:        "subject is not html escaped",
			subject:     "{{.name}}",
			body:        "<p>ignored</p>",
			receiver:    Receiver{"name": "Smith & Co"},
			wantSubject: "Smith & Co",
			wantBody:    "<p>ignored</p>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subjectTmpl, bodyTmpl, err := compile(Batch{Subject: tc.subject, Body: tc.body})
			if err != nil {
				t.Fatalf("compile: unexpected error: %v", err)
			}
			gotSubject, err := render(subjectTmpl, tc.receiver)
			if err != nil {
				t.Fatalf("render subject: unexpected error: %v", err)
			}
			if gotSubject != tc.wantSubject {
				t.Errorf("subject = %q, want %q", gotSubject, tc.wantSubject)
			}
			gotBody, err := render(bodyTmpl, tc.receiver)
			if err != nil {
				t.Fatalf("render body: unexpected error: %v", err)
			}
			if gotBody != tc.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tc.wantBody)
			}
		})
	}
}

func TestBodyIsHTMLEscaped(t *testing.T) {
	_, bodyTmpl, err := compile(Batch{Subject: "irrelevant", Body: "<p>{{.name}}</p>"})
	if err != nil {
		t.Fatalf("compile: unexpected error: %v", err)
	}
	got, err := render(bodyTmpl, Receiver{"name": "<script>alert(1)</script>"})
	if err != nil {
		t.Fatalf("render body: unexpected error: %v", err)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("body was not escaped, got raw script tag: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("body does not contain escaped script tag: %q", got)
	}
}

func TestCompileMalformedTemplateReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
	}{
		{name: "malformed subject", subject: "{{if}}", body: "fine"},
		{name: "malformed body", subject: "fine", body: "{{if .vip}}unterminated"},
		{name: "unknown function", subject: "{{nosuchfunc .name}}", body: "fine"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("compile panicked: %v", r)
				}
			}()
			_, _, err := compile(Batch{Subject: tc.subject, Body: tc.body})
			if err == nil {
				t.Fatalf("expected an error for malformed template, got nil")
			}
		})
	}
}

func TestPreprocessTemplateLeavesCommentsAlone(t *testing.T) {
	src := "{{/* {{name}} is a comment */}}{{name}}"
	got := preprocessTemplate(src)
	want := "{{/* {{name}} is a comment */}}{{.name}}"
	if got != want {
		t.Errorf("preprocessTemplate(%q) = %q, want %q", src, got, want)
	}
}

func TestPreprocessTemplateLeavesMultiTokenActionsAlone(t *testing.T) {
	tests := []string{
		`{{index . "first name"}}`,
		`{{if .vip}}`,
		`{{range .items}}`,
		`{{template "name" .}}`,
	}
	for _, src := range tests {
		if got := preprocessTemplate(src); got != src {
			t.Errorf("preprocessTemplate(%q) = %q, want unchanged", src, got)
		}
	}
}
