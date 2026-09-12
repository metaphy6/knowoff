package economy

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// Provider objects may gain new fields; duplicate (including case aliases),
// trailing, malformed, or excessive-depth JSON is still ambiguous and refused.
func billingJSON(raw []byte, dst any) error {
	if !utf8.Valid(raw) {
		return ErrBillingProof
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if billingJSONValue(d, 0) != nil {
		return ErrBillingProof
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrBillingProof
	}
	if json.Unmarshal(raw, dst) != nil {
		return ErrBillingProof
	}
	return nil
}

// DecodeReceipt is the strict public request boundary. Provider response
// objects may add fields; a caller cannot add account/amount/expiry authority.
func DecodeReceipt(raw []byte, limit int64) (ReceiptRequest, error) {
	var req ReceiptRequest
	if limit < 1 || int64(len(raw)) > limit || billingJSON(raw, &req) != nil {
		return req, ErrBillingProof
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil {
		return req, ErrBillingProof
	}
	var err error
	switch req.Platform {
	case PlatformGooglePlay:
		_, err = googleReceiptToken(req)
	case PlatformAppStore:
		_, err = appleReceiptID(req)
	default:
		err = ErrBillingProof
	}
	if err != nil {
		return ReceiptRequest{}, ErrBillingProof
	}
	return req, nil
}
func billingJSONValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrBillingProof
	}
	token, e := d.Token()
	if e != nil {
		return ErrBillingProof
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			s, ok := key.(string)
			s = strings.ToLower(s)
			if e != nil || !ok || seen[s] {
				return ErrBillingProof
			}
			seen[s] = true
			if billingJSONValue(d, depth+1) != nil {
				return ErrBillingProof
			}
		}
		last, e := d.Token()
		if e != nil || last != json.Delim('}') {
			return ErrBillingProof
		}
	case '[':
		for d.More() {
			if billingJSONValue(d, depth+1) != nil {
				return ErrBillingProof
			}
		}
		last, e := d.Token()
		if e != nil || last != json.Delim(']') {
			return ErrBillingProof
		}
	default:
		return ErrBillingProof
	}
	return nil
}
