package jserial

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if err := initSerializedObjects(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load serialized object fixtures: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// -------------------------------- //
// -- Begin Negative Tests Cases -- //
// -------------------------------- //

const (
	streamMagic      = "aced"
	streamVersion    = "0005"
	tcNull           = "70"
	tcReference      = "71"
	tcClassDesc      = "72"
	tcObject         = "73"
	tcString         = "74"
	tcArray          = "75"
	tcClass          = "76"
	tcBlockData      = "77"
	tcEndBlockData   = "78"
	tcReset          = "79"
	tcBlockDataLong  = "7a"
	tcException      = "7b"
	tcLongString     = "7c"
	tcProxyClassDesc = "7d"
	tcEnum           = "7e"
	baseWireHandle   = "7e0000"
	scWriteMethod    = "01"
	scBlockData      = "08"
	scSerializable   = "02"
	scExternalizable = "04"
	scEnum           = "10"

	streamPrefix = streamMagic + streamVersion + tcObject
	serialVer    = "1234567887654321"
)

var (
	strValAsHex  = hex.EncodeToString([]byte("abcdefg"))
	someClassEnc = encodeStr("SomeClass")
	fooEnc       = encodeStr("foo")
)

func encodeStr(s string) string {
	hexStr := hex.EncodeToString([]byte(s))
	return fmt.Sprintf("%04x%s", uint16(len(hexStr)>>1), hexStr)
}

func streamHex(overrideName, overrideVal string) string {

	vals := map[string]string{
		"flags":     scSerializable,
		"fieldType": hex.EncodeToString([]byte("I")),
		"classDesc": tcClassDesc,
	}

	if overrideName == "fieldType" {
		vals["fieldType"] = hex.EncodeToString([]byte(overrideVal))
	} else {
		vals[overrideName] = overrideVal
	}

	return streamPrefix + vals["classDesc"] + someClassEnc + serialVer + vals["flags"] + "0001" + vals["fieldType"] +
		fooEnc + tcEndBlockData + tcNull + "01234567"

}

func getErr(hexStr string) (err error) {
	var b []byte
	if b, err = hex.DecodeString(hexStr); err != nil {
		return
	}
	_, err = ParseSerializedObjectMinimal(b)
	return
}

func TestBadMagicValue(t *testing.T) {
	err := getErr("acde0005")
	if err == nil || !strings.Contains(err.Error(), "STREAM_MAGIC") {
		t.Fail()
	}
}

func TestBadVersion(t *testing.T) {
	err := getErr("aced0004")
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Fail()
	}
}

func TestStringTooLong(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcLongString + "7000000000000000" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), "string larger than") {
		t.Fail()
	}
}

func TestStringPrematureEnd(t *testing.T) {
	expectedErrStr := "premature end"
	err := getErr(streamMagic + streamVersion + tcString + "0008" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), expectedErrStr) {
		t.Fail()
	}
	expectedErrStr = "block data exceeds size"
	err = getErr(streamMagic + streamVersion + tcString + "00" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), expectedErrStr) {
		t.Fail()
	}
}

func TestResetNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcReset)
	if err == nil || !strings.Contains(err.Error(), "parsing Reset") {
		t.Fail()
	}
}

func TestExceptionNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcException)
	if err == nil || !strings.Contains(err.Error(), "parsing Exception") {
		t.Fail()
	}
}

func TestProxyClassDescNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcProxyClassDesc)
	if err == nil || !strings.Contains(err.Error(), "parsing ProxyClassDesc") {
		t.Fail()
	}
}

func TestUnkownType(t *testing.T) {
	err := getErr(streamMagic + streamVersion + "67")
	if err == nil || !strings.Contains(err.Error(), "unknown type 0x67") {
		t.Fail()
	}
}

func TestBadFlags(t *testing.T) {
	err := getErr(streamHex("flags", "00"))
	if err == nil || !strings.Contains(err.Error(), "flags 0x0") {
		t.Fail()
	}
}

func TestV1Extern(t *testing.T) {
	err := getErr(streamHex("flags", scExternalizable))
	if err == nil || !strings.Contains(err.Error(), "version 1 external") {
		t.Fail()
	}
}

func TestUnkownPrimitive(t *testing.T) {
	err := getErr(streamHex("fieldType", "Q"))
	if err == nil || !strings.Contains(err.Error(), "field type 'Q'") {
		t.Fail()
	}
}

func TestBadClassDesc(t *testing.T) {
	err := getErr(streamHex("classDesc", tcObject))
	if err == nil || !strings.Contains(err.Error(), "Object not allowed") {
		t.Fail()
	}
}

func TestWrongHashSetSize(t *testing.T) {
	hexStr := streamPrefix + tcClassDesc + encodeStr("java.util.HashSet") + "ba44859596b8b734" + "03" + "0000" +
		tcEndBlockData + tcNull + tcBlockData + "0c" + "00000003" + "00000000" + "00000003" + tcString + fooEnc +
		tcEndBlockData
	err := getErr(hexStr)
	if err == nil || !strings.Contains(err.Error(), "incorrect number of elements") {
		t.Fail()
	}
}

// An empty class descriptor name used to be accepted silently, because the
// validation wrapped a nil error. Accepting it also skipped the newHandle
// call, desyncing every later reference in the stream.
func TestEmptyClassName(t *testing.T) {
	hexStr := streamMagic + streamVersion + tcObject + tcClassDesc + encodeStr("") + serialVer +
		scSerializable + "0000" + tcEndBlockData + tcNull

	err := getErr(hexStr)
	if err == nil {
		t.Fatal("expected an error for an empty class name, got nil")
	}

	if !strings.Contains(err.Error(), "invalid class name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A one character class name such as "X" is valid Java and must be accepted.
// Only an empty name is rejected.
func TestSingleCharacterClassNameIsValid(t *testing.T) {
	hexStr := streamMagic + streamVersion + tcObject + tcClassDesc + encodeStr("X") + serialVer +
		scSerializable + "0001" + hex.EncodeToString([]byte("I")) + fooEnc + tcEndBlockData + tcNull + "0000002a"

	obj, err := ParseSerializedObjectMinimal(mustDecodeHex(t, hexStr))
	if err != nil {
		t.Fatalf("a one character class name must parse: %v", err)
	}

	expected := map[string]interface{}{"foo": int32(42)}
	if !reflect.DeepEqual(obj[0], expected) {
		t.Fatalf("unexpected object: got %#v, want %#v", obj[0], expected)
	}
}

// parseArray indexes the second byte of the class name to find the element
// type, so a descriptor that is too short or lacks the '[' prefix must error
// rather than panic.
func TestInvalidArrayClassName(t *testing.T) {
	for _, name := range []string{"[", "X", "Xy"} {
		hexStr := streamMagic + streamVersion + tcArray + tcClassDesc + encodeStr(name) +
			serialVer + scSerializable + "0000" + tcEndBlockData + tcNull + "00000000"

		err := getErr(hexStr)
		if err == nil {
			t.Fatalf("expected an error for array class name %q, got nil", name)
		}

		if !strings.Contains(err.Error(), "invalid array class name") {
			t.Fatalf("unexpected error for array class name %q: %v", name, err)
		}
	}
}

// mustDecodeHex decodes a hex encoded stream for tests that need the parsed
// value rather than just the error.
func mustDecodeHex(t *testing.T, hexStr string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("decode hex stream: %v", err)
	}

	return decoded
}

// A reference to a handle that was never assigned used to yield a nil object
// with no error, which is indistinguishable from a legitimately null field.
func TestInvalidReferenceHandle(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcReference + "7e00ffff")
	if err == nil {
		t.Fatal("expected an error for an out-of-range reference, got nil")
	}

	if !strings.Contains(err.Error(), "invalid reference") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A reference below the base wire handle is equally invalid.
func TestReferenceBelowBaseHandle(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcReference + "00000000")
	if err == nil {
		t.Fatal("expected an error for a reference below the base handle, got nil")
	}

	if !strings.Contains(err.Error(), "invalid reference") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertParseError runs one parse entry point and requires it to fail with a
// message containing want.
func assertParseError(t *testing.T, api, want string, parse func() error) {
	t.Helper()

	err := parse()
	if err == nil {
		t.Fatalf("%s: expected an error, got nil", api)
	}

	if !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: expected error containing %q, got: %v", api, want, err)
	}
}

// A TC_NULL class descriptor is only meaningful in the superclass position.
// Anywhere else it used to yield a plausible looking value with a nil error:
// an object became {}, which is indistinguishable from a genuinely empty map;
// an array became nil; an enum became its bare constant name; and a class
// handed back a typed nil *clazz, leaking an unexported type to the caller.
func TestNullClassDescriptorRejected(t *testing.T) {
	cases := []struct {
		name      string
		hexStr    string
		wantError string
	}{
		{
			"object",
			streamMagic + streamVersion + tcObject + tcNull,
			"object class descriptor is null",
		},
		{
			"array",
			streamMagic + streamVersion + tcArray + tcNull + "00000003",
			"array class descriptor is null",
		},
		{
			"enum",
			streamMagic + streamVersion + tcEnum + tcNull + tcString + encodeStr("ONE"),
			"enum class descriptor is null",
		},
		{
			"class",
			streamMagic + streamVersion + tcClass + tcNull,
			"class descriptor is null",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stream := mustDecodeHex(t, testCase.hexStr)

			assertParseError(t, "ParseSerializedObjectMinimal", testCase.wantError, func() error {
				_, err := ParseSerializedObjectMinimal(stream)

				return err
			})

			assertParseError(t, "ParseSerializedObject", testCase.wantError, func() error {
				_, err := ParseSerializedObject(stream)

				return err
			})

			assertParseError(t, "stream ParseSerializedObjectMinimal", testCase.wantError, func() error {
				sop := NewSerializedObjectParser(bytes.NewReader(stream), SetMaxDataBlockSize(len(stream)))
				_, err := sop.ParseSerializedObjectMinimal()

				return err
			})

			assertParseError(t, "stream ParseSerializedObject", testCase.wantError, func() error {
				sop := NewSerializedObjectParser(bytes.NewReader(stream), SetMaxDataBlockSize(len(stream)))
				_, err := sop.ParseSerializedObject()

				return err
			})
		})
	}
}

// The legitimate use of TC_NULL in the classDesc position is the superclass of
// a class at the top of its hierarchy. The checks above must not reject it.
func TestNullSuperClassDescriptorIsValid(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(mustDecodeHex(t, streamHex("flags", scSerializable)))
	if err != nil {
		t.Fatalf("a null superclass descriptor must parse: %v", err)
	}

	expected := map[string]interface{}{"foo": int32(0x01234567)}
	if !reflect.DeepEqual(obj[0], expected) {
		t.Fatalf("unexpected object: got %#v, want %#v", obj[0], expected)
	}
}

// hashMapStreamHex builds a HashMap stream with an arbitrary declared size and
// an arbitrary run of encoded entries, so the two can be made to disagree.
func hashMapStreamHex(declaredSizeHex, entries string) string {
	return streamPrefix + tcClassDesc + encodeStr("java.util.HashMap") + "0507dac1c31660d1" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "08" + "00000010" + declaredSizeHex + entries + tcEndBlockData
}

// enumMapStreamHex is the equivalent for EnumMap, whose size sits at offset 0.
func enumMapStreamHex(declaredSizeHex, entries string) string {
	return streamPrefix + tcClassDesc + encodeStr("java.util.EnumMap") + "065d7df7be907ca1" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "04" + declaredSizeHex + entries + tcEndBlockData
}

func strPairHex(key, value string) string {
	return tcString + encodeStr(key) + tcString + encodeStr(value)
}

// A declared entry count that is negative, or merely not greater than the
// encoded pair count, used to be accepted and then produce a silently
// truncated or empty map with a nil error. An empty map is a meaningful value
// to callers, so it must never stand in for malformed metadata.
func TestMapEntryCountMustMatchExactly(t *testing.T) {
	onePair := strPairHex("k", "v")
	twoPairs := onePair + strPairHex("k2", "v2")

	cases := []struct {
		name      string
		sizeHex   string
		entries   string
		wantError string
	}{
		{"zero declared, none encoded", "00000000", "", ""},
		{"two declared, two encoded", "00000002", twoPairs, ""},
		{"zero declared, two encoded", "00000000", twoPairs, "declared 0, encoded 2"},
		{"one declared, two encoded", "00000001", twoPairs, "declared 1, encoded 2"},
		{"three declared, two encoded", "00000003", twoPairs, "declared 3, encoded 2"},
		{"negative declared, none encoded", "ffffffff", "", "negative map size"},
		{"negative declared, one encoded", "ffffffff", onePair, "negative map size"},
		{"dangling element", "00000001", onePair + tcString + encodeStr("dangle"),
			"do not form whole key/value pairs"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := getErr(hashMapStreamHex(testCase.sizeHex, testCase.entries))

			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("expected a valid map, got: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("expected an error for mismatched entry count, got nil")
			}

			if !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("expected error containing %q, got: %v", testCase.wantError, err)
			}
		})
	}
}

// enumMapPostProc carried the identical flaw and gets the identical check.
func TestEnumMapEntryCountMustMatchExactly(t *testing.T) {
	cases := []struct {
		name      string
		sizeHex   string
		entries   string
		wantError string
	}{
		{"negative declared", "ffffffff", "", "negative enum map size"},
		{"zero declared, one encoded", "00000000", strPairHex("k", "v"),
			"declared 0, encoded 1"},
		{"two declared, one encoded", "00000002", strPairHex("k", "v"),
			"declared 2, encoded 1"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := getErr(enumMapStreamHex(testCase.sizeHex, testCase.entries))
			if err == nil {
				t.Fatal("expected an error for mismatched entry count, got nil")
			}

			if !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("expected error containing %q, got: %v", testCase.wantError, err)
			}
		})
	}
}

// arrayListStreamHex builds an ArrayList stream where the `size` field and the
// element count at the head of the block data can be made to disagree.
func arrayListStreamHex(sizeFieldHex, declaredHex, elements string) string {
	return streamPrefix + tcClassDesc + encodeStr("java.util.ArrayList") + "7881d21d99c7619d" +
		"03" + "0001" + hex.EncodeToString([]byte("I")) + encodeStr("size") +
		tcEndBlockData + tcNull +
		sizeFieldHex + tcBlockData + "04" + declaredHex + elements + tcEndBlockData
}

// ArrayList.writeObject emits the element count twice, as the `size` field and
// again at the head of its block data. A stream where they disagree is corrupt,
// and used to decode as a plausible list -- a declared size of 5 with zero
// encoded elements produced an empty list with a nil error.
func TestListSizeFieldMustMatchElementCount(t *testing.T) {
	one := tcString + encodeStr("a")
	two := one + tcString + encodeStr("b")

	cases := []struct {
		name      string
		sizeField string
		declared  string
		elements  string
		wantError bool
	}{
		{"empty and consistent", "00000000", "00000000", "", false},
		{"two and consistent", "00000002", "00000002", two, false},
		{"field overstates, none encoded", "00000005", "00000000", "", true},
		{"field understates", "00000000", "00000001", one, true},
		{"field overstates, one encoded", "00000009", "00000001", one, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := getErr(arrayListStreamHex(testCase.sizeField, testCase.declared, testCase.elements))

			if !testCase.wantError {
				if err != nil {
					t.Fatalf("expected a valid list, got: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("expected an error for a size field that disagrees with the element count, got nil")
			}

			if !strings.Contains(err.Error(), "does not match encoded element count") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ArrayDeque uses the same post-processor but declares its fields transient, so
// it contributes no `size` field. The cross-check must not reject it.
func TestArrayDequeHasNoSizeFieldToCrossCheck(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrayDeque"])
	if err != nil {
		t.Fatalf("parse arrayDeque: %v", err)
	}

	expected := []interface{}{"foo", int32(123)}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fatalf("unexpected deque: got %#v, want %#v", obj[1], expected)
	}
}

// listPostProc and hashSetPostProc already compare the declared count exactly
// with len(data) != size+1, which also rejects a negative size. Pin that, so
// the two never drift toward the looser comparison the pair based handlers had.
func TestListAndSetSizesRejectNegative(t *testing.T) {
	setHex := streamPrefix + tcClassDesc + encodeStr("java.util.HashSet") + "ba44859596b8b734" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "0c" + "00000010" + "00000000" + "ffffffff" + tcEndBlockData

	if err := getErr(setHex); err == nil {
		t.Fatal("expected an error for a negative set size, got nil")
	}

	listHex := streamPrefix + tcClassDesc + encodeStr("java.util.ArrayList") + "7881d21d99c7619d" +
		"03" + "0001" + hex.EncodeToString([]byte("I")) + encodeStr("size") +
		tcEndBlockData + tcNull +
		"ffffffff" + tcBlockData + "04" + "ffffffff" + tcEndBlockData

	if err := getErr(listHex); err == nil {
		t.Fatal("expected an error for a negative list size, got nil")
	}
}

// A map whose key is a reference to an unassigned handle used to decode as an
// empty map with no error, so a non-empty serialized map could disappear
// without any signal to the caller.
func TestMapWithInvalidReferenceKey(t *testing.T) {
	// flags 03 is SC_SERIALIZABLE|SC_WRITE_METHOD as a single byte, then a
	// block of bucket count and size 1, then one key/value pair.
	hexStr := streamPrefix + tcClassDesc + encodeStr("java.util.HashMap") + "0507dac1c31660d1" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "08" + "00000010" + "00000001" +
		tcReference + "7e00ffff" +
		tcString + encodeStr("somevalue") +
		tcEndBlockData

	err := getErr(hexStr)
	if err == nil {
		t.Fatal("expected an error for a map key referencing an unassigned handle, got nil")
	}

	if !strings.Contains(err.Error(), "invalid reference") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A set member that is not a string used to be skipped, so a non-empty set
// could decode as a smaller set, or as an empty one.
func TestNonStringSetMember(t *testing.T) {
	// block of capacity, load factor and size 1, then a single null member
	hexStr := streamPrefix + tcClassDesc + encodeStr("java.util.HashSet") + "ba44859596b8b734" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "0c" + "00000010" + "00000000" + "00000001" +
		tcNull + tcEndBlockData

	err := getErr(hexStr)
	if err == nil {
		t.Fatal("expected an error for a non-string set member, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported set member type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// An enum map key that is not an enum constant used to be skipped silently.
func TestNonEnumEnumMapKey(t *testing.T) {
	hexStr := streamPrefix + tcClassDesc + encodeStr("java.util.EnumMap") + "065d7df7be907ca1" +
		"03" + "0000" + tcEndBlockData + tcNull +
		tcBlockData + "04" + "00000001" +
		tcString + encodeStr("notanenum") +
		tcString + encodeStr("somevalue") +
		tcEndBlockData

	err := getErr(hexStr)
	if err == nil {
		t.Fatal("expected an error for a non-enum enum map key, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported enum map key type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The rejected key type must be named by its Java class, not by jserial's
// internal representation, so the error is actionable from a log line.
func TestNonStringMapKeyNamesJavaType(t *testing.T) {
	_, err := ParseSerializedObjectMinimal(objs["hashMapObj"])
	if err == nil {
		t.Fatal("expected an error for a non-string map key, got nil")
	}

	if !strings.Contains(err.Error(), "java.lang.Integer") {
		t.Fatalf("error should name the Java key type, got: %v", err)
	}
}

// Valid back references must keep working: the dupe fixture writes the same
// object three times and relies on the handle table resolving them.
func TestValidReferencesStillResolve(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["dupe"])
	if err != nil {
		t.Fatalf("parse dupe: %v", err)
	}

	if len(obj) != 5 {
		t.Fatalf("unexpected object count: got %d, want 5", len(obj))
	}

	if !reflect.DeepEqual(obj[1], obj[3]) {
		t.Fatalf("reference did not resolve to the same value: %#v vs %#v", obj[1], obj[3])
	}
}

// -------------------------------- //
// -- Begin Positive Tests Cases -- //
// -------------------------------- //

//go:embed testdata/serialized_objects.json
var serializedObjectsJSON []byte

var objs map[string][]byte

func initSerializedObjects() error {
	expectedKeys := []string{
		"canary",
		"string",
		"longStr",
		"null",
		"dupe",
		"prim",
		"boxedPrim",
		"inherited",
		"dupeField",
		"primArray",
		"nestedArr",
		"arrFields",
		"enum",
		"exception",
		"custom",
		"extern",
		"longExtern",
		"hashMapStr",
		"hashMapObj",
		"hashMapEmpty",
		"hashTblStr",
		"enumMap",
		"arrayList",
		"stringListMapHolder",
		"arrayDeque",
		"hashSet",
		"date",
	}

	var encodedObjs map[string]string
	if err := json.Unmarshal(serializedObjectsJSON, &encodedObjs); err != nil {
		return fmt.Errorf("unmarshal embedded serialized object fixtures: %w", err)
	}

	expected := make(map[string]struct{}, len(expectedKeys))
	for _, key := range expectedKeys {
		expected[key] = struct{}{}
		if _, ok := encodedObjs[key]; !ok {
			return fmt.Errorf("validate embedded serialized object fixtures: missing key %q", key)
		}
	}
	for key := range encodedObjs {
		if _, ok := expected[key]; !ok {
			return fmt.Errorf("validate embedded serialized object fixtures: unexpected key %q", key)
		}
	}

	decodedObjs := make(map[string][]byte, len(encodedObjs))
	for _, key := range expectedKeys {
		decoded, err := base64.StdEncoding.Strict().DecodeString(encodedObjs[key])
		if err != nil {
			return fmt.Errorf("decode embedded serialized object fixture %q as standard Base64: %w", key, err)
		}
		decodedObjs[key] = decoded
	}

	objs = decodedObjs
	return nil
}

func prettyPrint(t *testing.T, deserializedContent []interface{}) {
	jsonStr, err := json.MarshalIndent(deserializedContent, "", "    ")
	if err != nil {
		t.Fatalf("failed to pretty print deserialized content JSON: %+v", err)
	}
	fmt.Println(string(jsonStr))
}

func TestDeserialize(t *testing.T) {
	if _, err := ParseSerializedObjectMinimal(objs["canary"]); err != nil {
		t.Fail()
	}
}

func TestDeserializeString(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["string"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	s, isString := obj[1].(string)
	if !isString || s != "sometext" {
		t.Fail()
	}
}

func TestDeserializeLongString(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["longStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	s, isString := obj[1].(string)
	if !isString || s != strings.Repeat("x", 131072) {
		t.Fail()
	}
}

func TestDeserializeNull(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["null"])
	if err != nil || len(obj) != 3 || obj[1] != nil {
		t.Fail()
	}
}

func TestDeserializeDuplicate(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["dupe"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	if !reflect.DeepEqual(obj[1], obj[3]) {
		t.Fail()
	}
}

func TestDeserializePrimitives(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["prim"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	expected := map[string]interface{}{
		"i":  int32(-123),
		"s":  int16(-456),
		"l":  int64(-789),
		"by": int8(-21),
		"d":  float64(12.34),
		"f":  float32(76.5),
		"bo": true,
		"c":  "ሴ",
	}
	for k, v := range expected {
		if m[k] != v {
			t.Fail()
		}
	}
}

func TestDeserializeBoxedPrimitives(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["boxedPrim"])
	if err != nil || len(obj) != 10 {
		t.Fail()
	}
	expected := []interface{}{
		int32(-123),
		int16(-456),
		int64(-789),
		int8(-21),
		float64(12.34),
		float32(76.5),
		true,
		"ሴ",
	}
	for idx, v := range expected {
		if obj[idx+1] != v {
			t.Fail()
		}
	}
}

func TestDeserializeInherited(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["inherited"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["bar"] != int32(234) {
		t.Fail()
	}
	if m["foo"] != int32(123) {
		t.Fail()
	}
}

func TestDeserializeDuplicateField(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["dupeField"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["foo"] != int32(345) {
		t.Fail()
	}
}

func TestDeserializePrimitiveArray(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["primArray"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	arr, isArray := obj[1].([]interface{})
	if !isArray {
		t.Fail()
	}
	expected := []interface{}{
		int32(12),
		int32(34),
		int32(56),
	}
	for idx, v := range expected {
		if arr[idx] != v {
			t.Fail()
		}
	}
}

func TestDeserializeNestedArray(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["nestedArr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	arr, isArray := obj[1].([]interface{})
	if !isArray {
		t.Fail()
	}
	expected := []interface{}{
		[]interface{}{"a", "b"},
		[]interface{}{"c"},
	}
	for idx, v := range expected {
		if !reflect.DeepEqual(arr[idx], v) {
			t.Fail()
		}
	}
}

func TestDeserializeArrayFields(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrFields"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	expected := map[string]interface{}{
		"ia":  []interface{}{int32(12), int32(34), int32(56)},
		"iaa": []interface{}{[]interface{}{int32(11), int32(12)}, []interface{}{int32(21), int32(22), int32(23)}},
		"sa":  []interface{}{"foo", "bar"},
	}
	for k, v := range expected {
		if !reflect.DeepEqual(m[k], v) {
			t.Fail()
		}
	}
}

func TestDeserializeEnum(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["enum"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	expected := []interface{}{
		"ONE",
		"THREE",
		"THREE",
	}
	for idx, v := range expected {
		if obj[idx+1] != v {
			t.Fail()
		}
	}
}

func TestDeserializeException(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["exception"])
	//prettyPrint(t, obj)
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["detailMessage"] != "Kaboom" {
		t.Fail()
	}
}

func TestDeserializeCustom(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["custom"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["foo"] != int32(12345) {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes {
		t.Fail()
	}
	a0Expected, decodeErr := hex.DecodeString("b5eb2d00b5eb2d00b5eb2d")
	if decodeErr != nil || bytes.Compare(a0, a0Expected) != 0 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeExtern(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["extern"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes {
		t.Fail()
	}
	a0Expected, decodeErr := hex.DecodeString("0000000bb5eb2d00b5eb2d00b5eb2d")
	if decodeErr != nil || bytes.Compare(a0, a0Expected) != 0 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeLongExtern(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["longExtern"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes || len(a0) != 516 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeHashMapWithStrKeys(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashMapStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"bar": "baz",
		"foo": int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeHashMapWithObectKeys(t *testing.T) {
	// A key that is not a string cannot be represented in the decoded map, so
	// the parse fails instead of dropping the entry. Note that this rejects
	// the whole stream, including the Integer written after the map.
	_, err := ParseSerializedObjectMinimal(objs["hashMapObj"])
	if err == nil {
		t.Fatal("expected an error for a non-string map key, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported map key type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeserializeHashMapEmpty(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashMapEmpty"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap || len(m) != 0 {
		t.Fail()
	}
}

func TestDeserializeHashTableWithStringKeys(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashTblStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"bar": "baz",
		"foo": int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeEnumMap(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["enumMap"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"THREE": "baz",
		"ONE":   int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
	if obj[2] != "ONE" {
		t.Fail()
	}
	if obj[3] != "THREE" {
		t.Fail()
	}
}

func TestDeserializeArrayList(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrayList"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := []interface{}{"foo", int32(123)}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeStringListMapHolder(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["stringListMapHolder"])
	if err != nil || len(obj) != 3 {
		t.Fatalf("unexpected parse result: object count %d, error %v", len(obj), err)
	}
	expected := map[string]interface{}{
		"values": map[string]interface{}{
			"aa": []interface{}{"cc"},
			"bb": []interface{}{"dd", "ee"},
		},
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fatalf("unexpected holder: got %#v, want %#v", obj[1], expected)
	}
}

func TestDeserializeArrayDeque(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrayDeque"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := []interface{}{"foo", int32(123)}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeHashSet(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashSet"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]bool{
		"bar": true,
		"foo": true,
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeDate(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["date"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	if !reflect.DeepEqual(obj[1], time.Unix(403879620, 0)) {
		t.Fail()
	}
}
