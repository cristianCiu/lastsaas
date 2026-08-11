package quantity

import (
	"errors"
	"math"
	"testing"
)

func TestParseAndFormatExactQuantities(t *testing.T) {
	tests := map[string]Micros{"0": 0, "1": 1_000_000, "1.25": 1_250_000, "-0.000001": -1, " 12.340000 ": 12_340_000}
	for input, expected := range tests {
		actual, err := Parse(input)
		if err != nil || actual != expected {
			t.Fatalf("Parse(%q) = %d, %v; want %d", input, actual, err, expected)
		}
		if reparsed, err := Parse(actual.String()); err != nil || reparsed != actual {
			t.Fatalf("round trip %q: %d %v", actual.String(), reparsed, err)
		}
	}
	for _, input := range []string{"", "01", "1.0000001", "1e3", ".5", "1."} {
		if _, err := Parse(input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Parse(%q) error = %v, want ErrInvalid", input, err)
		}
	}
}

func TestCheckedArithmeticAndRationalConversion(t *testing.T) {
	if result, err := Convert(1_500_000, 2, 3); err != nil || result != 1_000_000 {
		t.Fatalf("exact conversion = %d, %v", result, err)
	}
	if _, err := Convert(1, 1, 3); !errors.Is(err, ErrInexact) {
		t.Fatalf("inexact conversion error = %v", err)
	}
	if _, err := Convert(1, 0, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid factor error = %v", err)
	}
	if _, err := Add(Micros(math.MaxInt64), 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("add overflow error = %v", err)
	}
	if _, err := Subtract(Micros(math.MinInt64), 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("subtract overflow error = %v", err)
	}
}
