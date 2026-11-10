package mailer

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildMessageNormalizesMailtoAddresses(t *testing.T) {
	receiver := Receiver{"email": " MAILTO:abc@xyz.com?subject=Ignored "}
	batch := Batch{SenderName: "Sender", SenderEmail: "mailto:sender@xyz.com"}
	message := buildMessage(batch, receiver, "Subject", "<p>Body</p>")
	if got := message.GetRecipients(); !reflect.DeepEqual(got, []string{"abc@xyz.com"}) {
		t.Errorf("recipients = %#v, want abc@xyz.com", got)
	}
	serialized := message.GetMessage()
	if strings.Contains(strings.ToLower(serialized), "mailto:") || !strings.Contains(serialized, "sender@xyz.com") {
		t.Errorf("message contains incorrect addresses: %s", serialized)
	}
	if receiver["email"] != " MAILTO:abc@xyz.com?subject=Ignored " {
		t.Errorf("building a message mutated the saved receiver: %#v", receiver)
	}
}
