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
	obj, err := ParseSerializedObjectMinimal(objs["hashMapObj"])
	if err != nil || len(obj) != 4 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"baz": "bar",
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
	if obj[2] != int32(123) {
		t.Fail()
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
