package v2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// Populate every field, including newly added fields, so a missing pointer or
// slice copy cannot hide behind a zero-valued hand-written fixture.
func cloneFixtureValue(t testing.TB, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			cloneFixtureValue(t, v.Field(i))
		}
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		cloneFixtureValue(t, v.Elem())
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		for i := 0; i < v.Len(); i++ {
			cloneFixtureValue(t, v.Index(i))
		}
	case reflect.String:
		v.SetString("İı <plain> & شاي 😀")
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint64:
		v.SetUint(9)
	case reflect.Bool:
		v.SetBool(true)
	default:
		t.Fatalf("new clone fixture field kind needs explicit coverage: %s", v.Type())
	}
}

func mutateCloneValue(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			mutateCloneValue(t, v.Field(i))
		}
	case reflect.Pointer:
		if !v.IsNil() {
			mutateCloneValue(t, v.Elem())
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			mutateCloneValue(t, v.Index(i))
		}
	case reflect.String:
		v.SetString("caller mutation")
	case reflect.Int, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Uint64:
		v.SetUint(v.Uint() + 1)
	case reflect.Bool:
		v.SetBool(!v.Bool())
	default:
		t.Fatalf("uncovered clone field %s", v.Type())
	}
}

func TestSnapshotClonePreservesBytesAndOwnsEveryMutableField(t *testing.T) {
	var populated Snapshot
	cloneFixtureValue(t, reflect.ValueOf(&populated).Elem())
	for name, original := range map[string]Snapshot{
		"populated": populated,
		"nil":       {},
		"empty":     {History: []PublicAction{}, Seats: []PublicSeat{}, Board: Board{Cards: []BoardCard{}}, ReadySeats: []int{}, Private: PrivateState{Hand: []Card{}, Capabilities: []ActionKind{}}},
	} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			copied := original.Clone()
			if !reflect.DeepEqual(original, copied) {
				t.Fatal("copy changed typed values or nil/empty representation")
			}
			wire, err := json.Marshal(copied)
			if err != nil || !bytes.Equal(before, wire) {
				t.Fatal("copy changed JSON bytes", err)
			}
			mutateCloneValue(t, reflect.ValueOf(&copied).Elem())
			after, err := json.Marshal(original)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("caller mutation reached original snapshot", err)
			}
		})
	}
}

func BenchmarkSnapshotClone(b *testing.B) {
	var original Snapshot
	cloneFixtureValue(b, reflect.ValueOf(&original).Elem())
	for len(original.History) < 128 {
		original.History = append(original.History, original.History...)
	}
	for _, method := range []string{"typed", "json_reference"} {
		b.Run(method, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var copied Snapshot
				if method == "typed" {
					copied = original.Clone()
				} else {
					raw, err := json.Marshal(original)
					if err != nil {
						b.Fatal(err)
					}
					if err := json.Unmarshal(raw, &copied); err != nil {
						b.Fatal(err)
					}
				}
				if len(copied.History) != 128 {
					b.Fatal(fmt.Sprint("lost history ", len(copied.History)))
				}
			}
		})
	}
}
