package jserial

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

//go:embed testdata/legacy_serialized_objects.json
var legacySerializedObjectsJSON []byte

func legacySerializedObject(t *testing.T, name string) []byte {
	t.Helper()

	var encoded map[string]string
	if err := json.Unmarshal(legacySerializedObjectsJSON, &encoded); err != nil {
		t.Fatalf("unmarshal legacy serialized object fixtures: %v", err)
	}

	expectedNames := []string{"exception", "hashSet"}
	if len(encoded) != len(expectedNames) {
		t.Fatalf("unexpected legacy fixture count: got %d, want %d", len(encoded), len(expectedNames))
	}
	for _, expectedName := range expectedNames {
		if _, exists := encoded[expectedName]; !exists {
			t.Fatalf("missing legacy fixture %q", expectedName)
		}
	}

	value, exists := encoded[name]
	if !exists {
		t.Fatalf("legacy fixture %q does not exist", name)
	}

	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		t.Fatalf("decode legacy fixture %q as standard Base64: %v", name, err)
	}
	return decoded
}

func TestDeserializeLegacyException(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(legacySerializedObject(t, "exception"))
	if err != nil {
		t.Fatalf("deserialize legacy exception: %v", err)
	}
	if len(obj) != 3 {
		t.Fatalf("unexpected object count: got %d, want 3", len(obj))
	}

	exception, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fatalf("unexpected legacy exception type: %T", obj[1])
	}
	if exception["detailMessage"] != "Kaboom" {
		t.Fatalf("unexpected detail message: got %#v, want %q", exception["detailMessage"], "Kaboom")
	}
}

func TestDeserializeLegacyHashSet(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(legacySerializedObject(t, "hashSet"))
	if err != nil {
		t.Fatalf("deserialize legacy HashSet: %v", err)
	}
	if len(obj) != 3 {
		t.Fatalf("unexpected object count: got %d, want 3", len(obj))
	}

	// The legacy stream contains "foo" and Integer(123). HashSet's minimal
	// representation intentionally contains only string members.
	expected := map[string]bool{"foo": true}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fatalf("unexpected legacy HashSet: got %#v, want %#v", obj[1], expected)
	}
}
