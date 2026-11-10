package mailer

import (
	"reflect"
	"testing"
)

func TestRowsToReceiversNormalizesHeadersWithoutChangingInput(t *testing.T) {
	rows := [][]string{
		{" Name ", " EMAIL ", "Department"},
		{" Ada ", " MAILTO:<ada@example.com>?subject=Hello ", " Engineering "},
		{" Grace ", " grace@example.com "},
		{"Skipped", "", "Unknown"},
	}
	want := []Receiver{
		{"Name": "Ada", "email": "ada@example.com", "Department": "Engineering"},
		{"Name": "Grace", "email": "grace@example.com"},
	}
	got, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("receivers = %#v, want %#v", got, want)
	}
	if rows[0][0] != " Name " || rows[0][1] != " EMAIL " {
		t.Fatalf("input headers were mutated: %#v", rows[0])
	}
}

func TestNormalizeEmail(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "mailto:abc@xyz.com", want: "abc@xyz.com"},
		{raw: " MAILTO:abc@xyz.com ", want: "abc@xyz.com"},
		{raw: "mailto:abc@xyz.com?subject=Hello&cc=other@xyz.com", want: "abc@xyz.com"},
		{raw: "mailto:<abc@xyz.com>", want: "abc@xyz.com"},
		{raw: " abc@xyz.com ", want: "abc@xyz.com"},
		{raw: "mailto:", want: ""},
	} {
		t.Run(test.raw, func(t *testing.T) {
			if got := normalizeEmail(test.raw); got != test.want {
				t.Errorf("normalizeEmail(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestParseReceiversMailto(t *testing.T) {
	receivers, err := parseReceivers("name,email\nAda,mailto:abc@xyz.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(receivers) != 1 || receivers[0]["email"] != "abc@xyz.com" {
		t.Fatalf("receivers = %#v, want abc@xyz.com", receivers)
	}
}

func TestParseReceiversValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", want: "receivers must be CSV with a header and at least one row"},
		{name: "header only", raw: "email", want: "receivers must be CSV with a header and at least one row"},
		{name: "malformed CSV", raw: "email,name\nada@example.com", want: "receivers must be CSV with a header and at least one row"},
		{name: "missing email column", raw: "name\nAda", want: "receiver CSV needs an email column"},
		{name: "blank emails", raw: "name,email\nAda,", want: "no valid receiver emails found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseReceivers(test.raw)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
