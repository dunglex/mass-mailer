package mailer

import (
	"encoding/csv"
	"errors"
	netmail "net/mail"
	"strings"
)

func parseReceivers(raw string) ([]Receiver, error) {
	rows, err := csv.NewReader(strings.NewReader(raw)).ReadAll()
	if err != nil {
		return nil, errors.New("receivers must be CSV with a header and at least one row")
	}
	return rowsToReceivers(rows)
}

func rowsToReceivers(rows [][]string) ([]Receiver, error) {
	if len(rows) < 2 {
		return nil, errors.New("receivers must be CSV with a header and at least one row")
	}
	// Normalize headers once, preserving the case of custom template fields.
	headers := make([]string, len(rows[0]))
	foundEmail := false
	for index, header := range rows[0] {
		header = strings.TrimSpace(header)
		if strings.EqualFold(header, "email") {
			header = "email"
			foundEmail = true
		}
		headers[index] = header
	}
	if !foundEmail {
		return nil, errors.New("receiver CSV needs an email column")
	}
	var receivers []Receiver
	for _, row := range rows[1:] {
		receiver := Receiver{}
		for index, value := range row {
			if index < len(headers) {
				header := headers[index]
				value = strings.TrimSpace(value)
				if header == "email" {
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
