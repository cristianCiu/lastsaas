package imports

import "testing"

func TestParseCSVMixedDelimitersAndMapping(t *testing.T) {
	doc, err := ParseCSV("code;name\nkg;Kilogram\n")
	if err != nil || len(doc.Rows) != 1 {
		t.Fatalf("ParseCSV() error=%v rows=%d", err, len(doc.Rows))
	}
	mapped, err := ApplyMapping(doc, map[string]string{"code": "code", "name": "name"}, []string{"code", "name"})
	if err != nil || mapped.Rows[0].Values["name"] != "Kilogram" {
		t.Fatalf("ApplyMapping() error=%v mapped=%v", err, mapped.Rows)
	}
}

func TestParseCSVRejectsInvalidUTF8AndLimits(t *testing.T) {
	if _, err := ParseCSV(string([]byte{'a', ',', 0xff})); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
	if _, err := ParseCSV("code\n" + string(make([]byte, MaxCSVBytes))); err == nil {
		t.Fatal("oversized CSV was accepted")
	} else if err.Error() != "CSV exceeds the 128 KiB limit" {
		t.Fatalf("unexpected size error: %v", err)
	}
}

func TestApplyMappingRejectsUnknownField(t *testing.T) {
	doc, err := ParseCSV("code\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMapping(doc, map[string]string{"secret": "code"}, []string{"code"}); err == nil {
		t.Fatal("unknown mapping field was accepted")
	}
}
