package imports

import (
	"fmt"
	"strings"
)

// MapRows applies the optional canonical-field-to-header mapping. Unknown
// mapping fields and headers are rejected so a typo cannot silently import the
// wrong values.
func MapRows(document CSVDocument, mapping map[string]string, fields []string) ([]CSVRow, error) {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for field, header := range mapping {
		if _, ok := allowed[field]; !ok {
			return nil, fmt.Errorf("unknown mapping field %q", field)
		}
		found := false
		for _, h := range document.Headers {
			if h == header {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("mapping header %q was not found", header)
		}
	}
	result := make([]CSVRow, len(document.Rows))
	for i, row := range document.Rows {
		result[i] = CSVRow{Number: row.Number, Values: make(map[string]string, len(fields))}
		for _, field := range fields {
			header := mapping[field]
			if header == "" {
				header = field
			}
			result[i].Values[field] = strings.TrimSpace(row.Values[header])
		}
	}
	return result, nil
}
