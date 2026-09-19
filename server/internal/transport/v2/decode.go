package v2

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

type Validatable interface{ Validate(Limits) error }

// Decode is strict at every nesting level: duplicate/unknown/case-altered keys,
// absent required keys, explicit nulls, fractional integers and trailing JSON
// are errors. Never reuse partially decoded state after an error.
func Decode(data []byte, dst Validatable, limits Limits) error {
	if err := limits.Validate(); err != nil {
		return err
	}
	if len(data) > limits.MaxFrameBytes {
		return invalid(ErrFrameTooLarge, "frame")
	}
	if !utf8.Valid(data) {
		return invalid(ErrMalformed, "utf8")
	}
	value := reflect.ValueOf(dst)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return invalid(ErrMalformed, "destination")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := uniqueValue(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return invalid(ErrMalformed, "trailing_json")
	}
	if err := shape(data, value.Type().Elem()); err != nil {
		return err
	}
	// Decode into fresh storage so omitted optional fields cannot retain secrets
	// from a previously decoded recipient or phase.
	fresh := reflect.New(value.Type().Elem())
	if err := json.Unmarshal(data, fresh.Interface()); err != nil {
		return invalid(ErrMalformed, "field_type")
	}
	validated, ok := fresh.Interface().(Validatable)
	if !ok {
		return invalid(ErrMalformed, "destination")
	}
	if err := validated.Validate(limits); err != nil {
		return err
	}
	value.Elem().Set(fresh.Elem())
	return nil
}

func uniqueValue(dec *json.Decoder, depth int) error {
	// Parser depth is an implementation safety bound, independent of gameplay.
	if depth > 32 {
		return invalid(ErrMalformed, "nesting")
	}
	token, err := dec.Token()
	if err != nil {
		return invalid(ErrMalformed, "json")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return invalid(ErrMalformed, "json")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return invalid(ErrMalformed, "duplicate_key")
			}
			seen[name] = true
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return invalid(ErrMalformed, "json")
	}
	if _, err := dec.Token(); err != nil {
		return invalid(ErrMalformed, "json")
	}
	return nil
}

func fields(t reflect.Type) map[string]reflect.StructField {
	out := map[string]reflect.StructField{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			for name, inner := range fields(f.Type) {
				out[name] = inner
			}
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			out[name] = f
		}
	}
	return out
}

func shape(data []byte, t reflect.Type) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return invalid(ErrMalformed, "null")
	}
	if t.Kind() == reflect.Pointer {
		return shape(data, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil || object == nil {
			return invalid(ErrMalformed, "object")
		}
		declared := fields(t)
		for name := range object {
			if _, ok := declared[name]; !ok {
				return invalid(ErrMalformed, "unknown_field")
			}
		}
		if t == reflect.TypeOf(Action{}) {
			if err := actionShape(object); err != nil {
				return err
			}
		}
		for name, f := range declared {
			raw, exists := object[name]
			if !exists {
				if !strings.Contains(f.Tag.Get("json"), ",omitempty") {
					return invalid(ErrMalformed, "missing_"+name)
				}
				continue
			}
			if err := shape(raw, f.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return invalid(ErrMalformed, "array")
		}
		for _, raw := range items {
			if err := shape(raw, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func actionShape(object map[string]json.RawMessage) error {
	var kind ActionKind
	if err := json.Unmarshal(object["kind"], &kind); err != nil {
		return invalid(ErrMalformed, "action_kind")
	}
	variants := map[ActionKind]string{
		ActionRespond: "copy_id", ActionPlace: "copy_id rating", ActionReplace: "copy_id slot",
		ActionOffer: "copy_id target_seat target_copy_id", ActionResolveOffer: "offer_id resolution",
		ActionTop: "copy_id target_copy_id", ActionDraw: "count", ActionVote: "target_seat",
		ActionPass: "", ActionReveal: "target_seat", ActionViewReveal: "", ActionFreeCard: "", ActionShuffle: "", ActionRevote: "",
		ActionReady: "", ActionPoke: "target_seat", ActionChat: "phrase_id text ui_locale",
	}
	allowed, known := variants[kind]
	if !known {
		return invalid(ErrInvalidAction, "variant")
	}
	keys := map[string]bool{"kind": true}
	for _, key := range strings.Fields(allowed) {
		keys[key] = true
	}
	for key := range object {
		if !keys[key] {
			return invalid(ErrInvalidAction, "contradictory_field")
		}
	}
	return nil
}
