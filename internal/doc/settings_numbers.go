package doc

import (
	"encoding/json"
	"math/big"
	"strings"
)

// sameSettingsValue compares values decoded by encoding/json with UseNumber.
// Numbers compare by exact value; objects ignore key order and arrays retain it.
func sameSettingsValue(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		xCoefficient, xExponent := settingsDecimal(x)
		yCoefficient, yExponent := settingsDecimal(y)
		return xExponent != nil && yExponent != nil && xCoefficient == yCoefficient && xExponent.Cmp(yExponent) == 0
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, value := range x {
			if !sameSettingsValue(value, y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for key, value := range x {
			other, present := y[key]
			if !present || !sameSettingsValue(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// settingsDecimal normalizes a native JSON number to a signed coefficient with
// no leading/trailing zeros and a base-ten exponent. Raw tokens remain untouched.
// Rat.SetString bounds exponents and expands powers; a big.Int exponent accepts
// valid JSON exponents without allocating memory proportional to their value.
// The input grammar has already been validated by encoding/json.
func settingsDecimal(number json.Number) (string, *big.Int) {
	text := number.String()
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(text), "e")
	exponent := new(big.Int)
	if hasExponent {
		if _, ok := exponent.SetString(exponentText, 10); !ok {
			return "", nil
		}
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	integer, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		return "0", new(big.Int)
	}
	coefficient := strings.TrimRight(digits, "0")
	exponent.Sub(exponent, big.NewInt(int64(len(fraction))))
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(coefficient))))
	if negative {
		coefficient = "-" + coefficient
	}
	return coefficient, exponent
}
