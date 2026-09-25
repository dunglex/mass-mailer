package mailer

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// receiversFromRequest determines the batch receivers for an incoming
// createBatch request. An uploaded file under the "receivers_file" form
// field takes precedence over the pasted "receivers" textarea; when no
// file was uploaded, it falls back to parsing the textarea as CSV.
func receiversFromRequest(r *http.Request) ([]Receiver, error) {
	file, header, err := r.FormFile("receivers_file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
			return parseReceivers(r.FormValue("receivers"))
		}
		return nil, fmt.Errorf("receivers file: %w", err)
	}
	defer file.Close()
	if header == nil || header.Size == 0 {
		// No real file was uploaded; fall back to the textarea.
		return parseReceivers(r.FormValue("receivers"))
	}
	return receiversFromUpload(file, header)
}

func receiversFromUpload(file multipart.File, header *multipart.FileHeader) ([]Receiver, error) {
	var rows [][]string
	var err error
	switch ext := strings.ToLower(filepath.Ext(header.Filename)); ext {
	case ".xlsx":
		rows, err = parseXLSX(file)
	case ".csv":
		rows, err = csv.NewReader(file).ReadAll()
	default:
		return nil, fmt.Errorf("unsupported receivers file type %q: only .xlsx and .csv are accepted", ext)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", header.Filename, err)
	}
	receivers, err := rowsToReceivers(rows)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", header.Filename, err)
	}
	return receivers, nil
}

// parseXLSX reads the first sheet of an XLSX workbook and returns its rows
// as raw string cells, ready for rowsToReceivers.
func parseXLSX(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("workbook has no sheets")
	}
	name := f.GetSheetName(0)
	rows, err := f.GetRows(name)
	if err != nil {
		return nil, err
	}
	for rowIndex, row := range rows {
		for colIndex := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				return nil, err
			}
			hasLink, link, err := f.GetCellHyperLink(name, cell)
			if err != nil {
				return nil, err
			}
			if hasLink {
				rows[rowIndex][colIndex] = link
			}
		}
	}

	// Drop trailing rows that are entirely empty.
	end := len(rows)
	for end > 0 && rowIsEmpty(rows[end-1]) {
		end--
	}
	return rows[:end], nil
}

func rowIsEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
