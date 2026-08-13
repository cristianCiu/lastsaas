package imports

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	// encoding/json can expand a byte (for example a control character) to a
	// six-byte \u00XX escape. This conservative cap keeps the encoded request
	// below the global 1 MiB body limit even in that worst case.
	MaxCSVBytes = 128 * 1024
	MaxCSVRows  = 5000
)

type CSVRow struct {
	Number int
	Values map[string]string
}

type CSVDocument struct {
	Headers []string
	Rows    []CSVRow
}

func ApplyMapping(document CSVDocument, mapping map[string]string, fields []string) (CSVDocument, error) {
	if len(mapping) == 0 {
		return document, nil
	}
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for field, header := range mapping {
		if _, ok := allowed[field]; !ok {
			return CSVDocument{}, fmt.Errorf("unknown mapping field %q", field)
		}
		if header == "" {
			return CSVDocument{}, fmt.Errorf("mapped CSV header for %q is empty", field)
		}
	}
	for _, field := range fields {
		header := mapping[field]
		if header == "" {
			continue
		}
		found := false
		for _, current := range document.Headers {
			if current == header {
				found = true
				break
			}
		}
		if !found {
			return CSVDocument{}, fmt.Errorf("mapped CSV header %q is missing", header)
		}
	}
	for i := range document.Rows {
		values := make(map[string]string, len(fields))
		for _, field := range fields {
			header := mapping[field]
			if header == "" {
				header = field
			}
			values[field] = document.Rows[i].Values[header]
		}
		document.Rows[i].Values = values
	}
	return document, nil
}

func detectDelimiter(data string) rune {
	line := data
	if index := strings.IndexByte(data, '\n'); index >= 0 {
		line = data[:index]
	}
	comma, semicolon := 0, 0
	inQuotes := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			if i+1 < len(line) && line[i+1] == '"' {
				i++
			} else {
				inQuotes = !inQuotes
			}
		case ',':
			if !inQuotes {
				comma++
			}
		case ';':
			if !inQuotes {
				semicolon++
			}
		}
	}
	if semicolon > comma {
		return ';'
	}
	return ','
}

func ParseCSV(content string) (CSVDocument, error) {
	if len(content) == 0 || len(content) > MaxCSVBytes {
		return CSVDocument{}, errors.New("CSV exceeds the 128 KiB limit")
	}
	if !utf8.ValidString(content) {
		return CSVDocument{}, errors.New("CSV must be valid UTF-8")
	}
	reader := csv.NewReader(strings.NewReader(content))
	reader.Comma = detectDelimiter(content)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = false
	values, err := reader.Read()
	if err != nil {
		return CSVDocument{}, errors.New("CSV header is required")
	}
	if len(values) == 0 {
		return CSVDocument{}, errors.New("CSV header is required")
	}
	headers := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for i, header := range values {
		headers[i] = strings.TrimSpace(header)
		if headers[i] == "" {
			return CSVDocument{}, fmt.Errorf("CSV header %d is empty", i+1)
		}
		if _, exists := seen[headers[i]]; exists {
			return CSVDocument{}, fmt.Errorf("duplicate CSV header %q", headers[i])
		}
		seen[headers[i]] = struct{}{}
	}
	document := CSVDocument{Headers: headers, Rows: make([]CSVRow, 0)}
	for rowNumber := 2; ; rowNumber++ {
		values, err = reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return CSVDocument{}, fmt.Errorf("CSV row %d is invalid", rowNumber)
		}
		if len(values) != len(headers) {
			return CSVDocument{}, fmt.Errorf("CSV row %d has the wrong number of columns", rowNumber)
		}
		if len(document.Rows) >= MaxCSVRows {
			return CSVDocument{}, errors.New("CSV exceeds the 5,000 data-row limit")
		}
		row := CSVRow{Number: rowNumber, Values: make(map[string]string, len(headers))}
		for i, header := range headers {
			row.Values[header] = strings.TrimSpace(values[i])
		}
		document.Rows = append(document.Rows, row)
	}
	return document, nil
}
