package game

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// Unlike a short gameplay trace, a shape-complete fixture exercises every map,
// pointer and slice, including optional outcome/offer/result-window state.
func visitCloneState(t testing.TB, v reflect.Value, populate bool) {
	t.Helper()
	if v.Type() == reflect.TypeFor[time.Time]() {
		v.Set(reflect.ValueOf(time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)))
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			visitCloneState(t, v.Field(i), populate)
		}
	case reflect.Pointer:
		if populate {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if !v.IsNil() {
			visitCloneState(t, v.Elem(), populate)
		}
	case reflect.Slice:
		if populate {
			v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		}
		for i := 0; i < v.Len(); i++ {
			visitCloneState(t, v.Index(i), populate)
		}
	case reflect.Map:
		if populate {
			v.Set(reflect.MakeMap(v.Type()))
			key := reflect.New(v.Type().Key()).Elem()
			value := reflect.New(v.Type().Elem()).Elem()
			visitCloneState(t, key, true)
			visitCloneState(t, value, true)
			v.SetMapIndex(key, value)
		} else {
			for _, key := range v.MapKeys() {
				value := reflect.New(v.Type().Elem()).Elem()
				value.Set(v.MapIndex(key))
				visitCloneState(t, value, false)
				v.SetMapIndex(key, value)
			}
		}
	case reflect.String:
		if populate {
			v.SetString("İı شاي <&> fixture")
		} else {
			v.SetString("uncommitted mutation")
		}
	case reflect.Int, reflect.Int64:
		if populate {
			v.SetInt(7)
		} else {
			v.SetInt(v.Int() + 1)
		}
	case reflect.Uint64:
		if populate {
			v.SetUint(11)
		} else {
			v.SetUint(v.Uint() + 1)
		}
	case reflect.Bool:
		v.SetBool(populate)
	default:
		t.Fatalf("new state field needs clone coverage: %s", v.Type())
	}
}

func TestTextStateCloneMatchesReferenceAndIsolatesPendingMutation(t *testing.T) {
	var original textState
	visitCloneState(t, reflect.ValueOf(&original).Elem(), true)
	for name, state := range map[string]*textState{"populated": &original, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			var reference textState
			if err := json.Unmarshal(before, &reference); err != nil {
				t.Fatal(err)
			}
			m := &TextMatch{state: state}
			copied, err := m.cloneState()
			if err != nil || !reflect.DeepEqual(copied, &reference) {
				t.Fatal("copy changed reference state", err)
			}
			visitCloneState(t, reflect.ValueOf(copied).Elem(), false)
			after, err := json.Marshal(m.state)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("uncommitted candidate mutation reached committed state", err)
			}
		})
	}
}

func TestTextStateClonePreservesOccurrenceClockIdentity(t *testing.T) {
	for _, at := range []time.Time{time.Now(), time.Date(2026, 9, 12, 12, 34, 56, 789, time.FixedZone("fixture", 3*60*60))} {
		m := &TextMatch{state: &textState{Result: &TextResult{OccurredAt: at}}}
		copied, err := m.cloneState()
		if err != nil || copied.Result.OccurredAt != at {
			t.Fatal("copy changed original occurrence time identity", err)
		}
		before, err := json.Marshal(m.state)
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(copied)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("copy changed durable occurrence serialization", err)
		}
	}
}

func BenchmarkTextStateClone(b *testing.B) {
	var original textState
	visitCloneState(b, reflect.ValueOf(&original).Elem(), true)
	for len(original.History) < 128 {
		original.History = append(original.History, original.History...)
	}
	m := &TextMatch{state: &original}
	for _, method := range []string{"current", "json_reference"} {
		b.Run(method, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var copied *textState
				if method == "current" {
					var err error
					copied, err = m.cloneState()
					if err != nil {
						b.Fatal(err)
					}
				} else {
					raw, err := json.Marshal(&original)
					if err != nil {
						b.Fatal(err)
					}
					if err := json.Unmarshal(raw, &copied); err != nil {
						b.Fatal(err)
					}
				}
				if len(copied.History) != 128 {
					b.Fatal("lost history", len(copied.History))
				}
			}
		})
	}
}
