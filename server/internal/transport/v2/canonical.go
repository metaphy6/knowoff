package v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// canonicalJSON is the v2 hash encoding: compact UTF-8; object keys sorted
// lexicographically (schema keys are ASCII); arrays retain order; numbers are
// integral decimal; no Unicode normalization. Strings escape only quote,
// backslash and U+0000..001F (short escapes for b/t/n/f/r, lowercase hex for the
// rest). HTML, slash, U+2028 and U+2029 remain literal. This is deliberately
// independent of Go struct field order and json.Marshal's HTML escaping.
func canonicalJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := writeCanonical(&out, normalized); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			return invalid(ErrMalformed, "nonintegral_hash_value")
		}
		out.WriteString(string(v))
	case string:
		out.WriteByte('"')
		for _, r := range v {
			switch r {
			case '"':
				out.WriteString(`\"`)
			case '\\':
				out.WriteString(`\\`)
			case '\b':
				out.WriteString(`\b`)
			case '\t':
				out.WriteString(`\t`)
			case '\n':
				out.WriteString(`\n`)
			case '\f':
				out.WriteString(`\f`)
			case '\r':
				out.WriteString(`\r`)
			default:
				if r < 0x20 {
					fmt.Fprintf(out, `\u%04x`, r)
				} else {
					out.WriteRune(r)
				}
			}
		}
		out.WriteByte('"')
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeCanonical(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return invalid(ErrMalformed, "hash_value")
	}
	return nil
}
