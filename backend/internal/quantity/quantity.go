package quantity

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const Scale int64 = 1_000_000

type Micros int64

var (
	ErrInvalid  = errors.New("invalid quantity")
	ErrOverflow = errors.New("quantity overflow")
	ErrInexact  = errors.New("inexact quantity conversion")
	decimal     = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]{1,6})?$`)
	maxInt64    = big.NewInt(int64(^uint64(0) >> 1))
	minInt64    = new(big.Int).Neg(new(big.Int).Add(maxInt64, big.NewInt(1)))
)

func Parse(value string) (Micros, error) {
	value = strings.TrimSpace(value)
	if !decimal.MatchString(value) {
		return 0, ErrInvalid
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 6-len(fraction))
	combined := strings.TrimLeft(parts[0]+fraction, "0")
	if combined == "" {
		return 0, nil
	}
	parsed, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return 0, ErrInvalid
	}
	if negative {
		parsed.Neg(parsed)
	}
	if parsed.Cmp(maxInt64) > 0 || parsed.Cmp(minInt64) < 0 {
		return 0, ErrOverflow
	}
	return Micros(parsed.Int64()), nil
}

func (value Micros) String() string {
	integer := big.NewInt(int64(value))
	negative := integer.Sign() < 0
	integer.Abs(integer)
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(integer, big.NewInt(Scale), fraction)
	result := whole.String()
	if fraction.Sign() != 0 {
		result += "." + strings.TrimRight(fmt.Sprintf("%06d", fraction.Int64()), "0")
	}
	if negative && value != 0 {
		result = "-" + result
	}
	return result
}

func Add(left, right Micros) (Micros, error) {
	return checked(new(big.Int).Add(big.NewInt(int64(left)), big.NewInt(int64(right))))
}

func Subtract(left, right Micros) (Micros, error) {
	return checked(new(big.Int).Sub(big.NewInt(int64(left)), big.NewInt(int64(right))))
}

func Convert(value Micros, numerator, denominator int64) (Micros, error) {
	if numerator <= 0 || denominator <= 0 {
		return 0, ErrInvalid
	}
	product := new(big.Int).Mul(big.NewInt(int64(value)), big.NewInt(numerator))
	result, remainder := new(big.Int), new(big.Int)
	result.QuoRem(product, big.NewInt(denominator), remainder)
	if remainder.Sign() != 0 {
		return 0, ErrInexact
	}
	return checked(result)
}

func checked(value *big.Int) (Micros, error) {
	if value.Cmp(maxInt64) > 0 || value.Cmp(minInt64) < 0 {
		return 0, ErrOverflow
	}
	return Micros(value.Int64()), nil
}
