package mailer

import (
	"bytes"
	"reflect"
	"sort"
	"testing"

	"github.com/xuri/excelize/v2"
)

func buildXLSX(t *testing.T, headers []string, data [][]string) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	for col, header := range headers {
		cell, err := excelize.CoordinatesToCellName(col+1, 1)
		if err != nil {
			t.Fatalf("coordinates: %v", err)
		}
		if err := f.SetCellValue(sheet, cell, header); err != nil {
			t.Fatalf("set header: %v", err)
		}
	}
	for rowIndex, row := range data {
		for col, value := range row {
			cell, err := excelize.CoordinatesToCellName(col+1, rowIndex+2)
			if err != nil {
				t.Fatalf("coordinates: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, value); err != nil {
				t.Fatalf("set cell: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return &buf
}

// sortedReceivers returns a copy of receivers sorted by email so comparisons
// are stable regardless of row order.
func sortedReceivers(receivers []Receiver) []Receiver {
	out := make([]Receiver, len(receivers))
	copy(out, receivers)
	sort.Slice(out, func(i, j int) bool { return out[i]["email"] < out[j]["email"] })
	return out
}

func TestXLSXMatchesCSVForEquivalentInput(t *testing.T) {
	headers := []string{"name", "email", "phone"}
	data := [][]string{
		{"Ada Lovelace", "ada@example.com", "+15550100"},
		{"Grace Hopper", "grace@example.com", "+15550101"},
	}

	buf := buildXLSX(t, headers, data)
	xlsxRows, err := parseXLSX(buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	xlsxReceivers, err := rowsToReceivers(xlsxRows)
	if err != nil {
		t.Fatalf("rowsToReceivers(xlsx): %v", err)
	}

	csvRaw := "name,email,phone\nAda Lovelace,ada@example.com,+15550100\nGrace Hopper,grace@example.com,+15550101\n"
	csvReceivers, err := parseReceivers(csvRaw)
	if err != nil {
		t.Fatalf("parseReceivers(csv): %v", err)
	}

	if !reflect.DeepEqual(sortedReceivers(xlsxReceivers), sortedReceivers(csvReceivers)) {
		t.Fatalf("xlsx receivers %#v do not match csv receivers %#v", xlsxReceivers, csvReceivers)
	}
}

func TestParseXLSXExtractsHyperlinkFromLabeledCell(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	if err := f.SetCellValue(sheet, "A1", "name"); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := f.SetCellValue(sheet, "B1", "email"); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := f.SetCellValue(sheet, "A2", "Ada Lovelace"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := f.SetCellValue(sheet, "B2", "Survey link"); err != nil {
		t.Fatalf("set label: %v", err)
	}
	const link = "https://example.com/survey"
	if err := f.SetCellHyperLink(sheet, "B2", link, "External"); err != nil {
		t.Fatalf("set hyperlink: %v", err)
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	rows, err := parseXLSX(&buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	if got := rows[1][1]; got != link {
		t.Fatalf("hyperlink cell = %q, want %q", got, link)
	}
}

func TestParseXLSXExtractsEmailFromMailtoHyperlink(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	if err := f.SetCellValue(sheet, "A1", "email"); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := f.SetCellValue(sheet, "A2", "Email Ada"); err != nil {
		t.Fatalf("set label: %v", err)
	}
	if err := f.SetCellHyperLink(sheet, "A2", "mailto:ada@example.com", "External"); err != nil {
		t.Fatalf("set hyperlink: %v", err)
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	rows, err := parseXLSX(&buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if got := receivers[0]["email"]; got != "ada@example.com" {
		t.Fatalf("email = %q, want %q", got, "ada@example.com")
	}
}

func TestParseXLSXNoEmailColumn(t *testing.T) {
	buf := buildXLSX(t, []string{"name", "phone"}, [][]string{{"Ada Lovelace", "+15550100"}})
	rows, err := parseXLSX(buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	if _, err := rowsToReceivers(rows); err == nil {
		t.Fatal("expected error for sheet with no email column")
	}
}

func TestParseXLSXSkipsBlankEmailRows(t *testing.T) {
	headers := []string{"name", "email"}
	data := [][]string{
		{"Ada Lovelace", "ada@example.com"},
		{"No Email", ""},
		{"Grace Hopper", "grace@example.com"},
	}
	buf := buildXLSX(t, headers, data)
	rows, err := parseXLSX(buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if len(receivers) != 2 {
		t.Fatalf("expected 2 receivers, got %d: %#v", len(receivers), receivers)
	}
	for _, receiver := range receivers {
		if receiver["email"] == "" {
			t.Fatalf("receiver with blank email was not skipped: %#v", receiver)
		}
	}
}

func TestRowsToReceiversRaggedRowsDoNotPanic(t *testing.T) {
	rows := [][]string{
		{"name", "email", "phone"},
		{"Ada Lovelace", "ada@example.com"}, // short row, missing phone
		{"Grace Hopper", "grace@example.com", "+15550101"},
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if len(receivers) != 2 {
		t.Fatalf("expected 2 receivers, got %d: %#v", len(receivers), receivers)
	}
	if _, ok := receivers[0]["phone"]; ok {
		t.Fatalf("short row should not have a phone key: %#v", receivers[0])
	}
}

func TestRowsToReceiversExtractsMailtoEmail(t *testing.T) {
	rows := [][]string{
		{"name", "email"},
		{"Ada Lovelace", "mailto:ada@example.com"},
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if got := receivers[0]["email"]; got != "ada@example.com" {
		t.Fatalf("email = %q, want %q", got, "ada@example.com")
	}
}

func TestParseXLSXIgnoresTrailingBlankRows(t *testing.T) {
	headers := []string{"name", "email"}
	data := [][]string{
		{"Ada Lovelace", "ada@example.com"},
		{"", ""},
		{"", ""},
	}
	buf := buildXLSX(t, headers, data)
	rows, err := parseXLSX(buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected trailing blank rows to be dropped, got %d rows: %#v", len(rows), rows)
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		t.Fatalf("rowsToReceivers: %v", err)
	}
	if len(receivers) != 1 {
		t.Fatalf("expected 1 receiver, got %d: %#v", len(receivers), receivers)
	}
}
