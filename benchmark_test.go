package jserial

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// sink keeps the compiler from eliminating parse results we do not inspect.
var sink []interface{}

//go:embed testdata/benchmark_fixtures.json
var stringListMapBenchmarkFixturesJSON []byte

type stringListMapBenchmarkCase struct {
	name         string
	keyCount     int
	valuesPerKey int
}

var stringListMapBenchmarkCases = []stringListMapBenchmarkCase{
	{name: "empty", keyCount: 0, valuesPerKey: 0},
	{name: "single", keyCount: 1, valuesPerKey: 1},
	{name: "medium", keyCount: 8, valuesPerKey: 2},
	{name: "large", keyCount: 64, valuesPerKey: 4},
}

func loadStringListMapBenchmarkFixtures(tb testing.TB) map[string][]byte {
	tb.Helper()

	var encoded map[string]string
	if err := json.Unmarshal(stringListMapBenchmarkFixturesJSON, &encoded); err != nil {
		tb.Fatalf("unmarshal string-list map benchmark fixtures: %v", err)
	}
	if len(encoded) != len(stringListMapBenchmarkCases) {
		tb.Fatalf("unexpected benchmark fixture count: got %d, want %d",
			len(encoded), len(stringListMapBenchmarkCases))
	}

	fixtures := make(map[string][]byte, len(encoded))
	for _, testCase := range stringListMapBenchmarkCases {
		value, exists := encoded[testCase.name]
		if !exists {
			tb.Fatalf("benchmark fixture %q does not exist", testCase.name)
		}

		decoded, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil {
			tb.Fatalf("decode benchmark fixture %q: %v", testCase.name, err)
		}
		fixtures[testCase.name] = decoded
	}

	return fixtures
}

// BenchmarkParseStringListMapMinimal measures the fully buffered minimal parse
// used for a serializable holder containing HashMap<String, ArrayList<String>>.
func BenchmarkParseStringListMapMinimal(b *testing.B) {
	fixtures := loadStringListMapBenchmarkFixtures(b)

	for _, testCase := range stringListMapBenchmarkCases {
		data := fixtures[testCase.name]
		valueCount := testCase.keyCount * testCase.valuesPerKey
		name := fmt.Sprintf("keys=%d/values=%d", testCase.keyCount, valueCount)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				content, err := ParseSerializedObjectMinimal(data)
				if err != nil {
					b.Fatalf("parse benchmark fixture %q: %v", testCase.name, err)
				}
				sink = content
			}
		})
	}
}

func TestStringListMapBenchmarkFixturesAreValid(t *testing.T) {
	fixtures := loadStringListMapBenchmarkFixtures(t)

	for _, testCase := range stringListMapBenchmarkCases {
		t.Run(testCase.name, func(t *testing.T) {
			content, err := ParseSerializedObjectMinimal(fixtures[testCase.name])
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if len(content) != 1 {
				t.Fatalf("unexpected top-level object count: got %d, want 1", len(content))
			}

			holder, ok := content[0].(map[string]interface{})
			if !ok || len(holder) != 1 {
				t.Fatalf("unexpected holder: %#v", content[0])
			}
			entries, ok := holder["values"].(map[string]interface{})
			if !ok {
				t.Fatalf("unexpected values field: %#v", holder["values"])
			}
			if len(entries) != testCase.keyCount {
				t.Fatalf("unexpected key count: got %d, want %d", len(entries), testCase.keyCount)
			}

			valueCount := 0
			for key, rawValues := range entries {
				if len(key) != 36 {
					t.Fatalf("unexpected key length: got %d, want 36", len(key))
				}
				values, ok := rawValues.([]interface{})
				if !ok || len(values) != testCase.valuesPerKey {
					t.Fatalf("unexpected values for key %q: %#v", key, rawValues)
				}
				for index, rawValue := range values {
					value, ok := rawValue.(string)
					if !ok {
						t.Fatalf("unexpected value type for key %q: %T", key, rawValue)
					}
					wantLength := 39
					if index%2 == 1 {
						wantLength = 48
					}
					if len(value) != wantLength {
						t.Fatalf("unexpected value length: got %d, want %d", len(value), wantLength)
					}
				}
				valueCount += len(values)
			}

			wantValueCount := testCase.keyCount * testCase.valuesPerKey
			if valueCount != wantValueCount {
				t.Fatalf("unexpected value count: got %d, want %d", valueCount, wantValueCount)
			}
		})
	}
}

// arraySizes is the element-count sweep used by the array benchmarks. Java
// payloads in the wild range from a handful of elements to multi-megabyte
// byte[] blobs, so the sweep spans both.
var arraySizes = []int{16, 1024, 65536}

func fixture(b *testing.B, name string) []byte {
	b.Helper()

	data, exists := objs[name]
	if !exists {
		b.Fatalf("serialized object fixture %q does not exist", name)
	}

	return data
}

// fixtureNames returns every embedded fixture name in a stable order.
func fixtureNames() []string {
	names := make([]string, 0, len(objs))
	for name := range objs {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// decodeStream converts a hex-encoded stream into bytes, panicking on
// malformed input since the benchmark stream builders are all static.
func decodeStream(hexStr string) []byte {
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(fmt.Sprintf("malformed benchmark stream: %v", err))
	}

	return decoded
}

// orFlags combines the single byte hex flag constants from the test suite
// into the one byte classDescFlags field the wire format expects.
func orFlags(flags ...string) string {
	var combined uint8

	for _, flag := range flags {
		decoded, err := hex.DecodeString(flag)
		if err != nil || len(decoded) != 1 {
			panic(fmt.Sprintf("malformed flag %q", flag))
		}

		combined |= decoded[0]
	}

	return fmt.Sprintf("%02x", combined)
}

// arrayHeader builds the stream prefix for a primitive or object array of the
// given class name and element count.
func arrayHeader(className string, size int) string {
	return streamMagic + streamVersion + tcArray + tcClassDesc + encodeStr(className) +
		serialVer + scSerializable + "0000" + tcEndBlockData + tcNull +
		fmt.Sprintf("%08x", uint32(size))
}

// byteArrayStream builds a stream holding a single byte[] of the given length.
func byteArrayStream(size int) []byte {
	stream := decodeStream(arrayHeader("[B", size))

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	return append(stream, payload...)
}

// intArrayStream builds a stream holding a single int[] of the given length.
func intArrayStream(size int) []byte {
	var sb strings.Builder

	sb.WriteString(arrayHeader("[I", size))

	for i := 0; i < size; i++ {
		fmt.Fprintf(&sb, "%08x", uint32(i))
	}

	return decodeStream(sb.String())
}

// stringArrayStream builds a stream holding a single String[] of the given
// length, which exercises the nested-content path rather than a primitive read.
func stringArrayStream(size int) []byte {
	var sb strings.Builder

	sb.WriteString(arrayHeader("[Ljava.lang.String;", size))

	for i := 0; i < size; i++ {
		sb.WriteString(tcString)
		sb.WriteString(encodeStr(fmt.Sprintf("element-%d", i)))
	}

	return decodeStream(sb.String())
}

// arrayListStream builds a java.util.ArrayList of strings large enough to
// measure the post-processing path, which the tiny embedded fixtures cannot.
func arrayListStream(size int) []byte {
	const (
		arrayListName   = "java.util.ArrayList"
		arrayListSerial = "7881d21d99c7619d"
		sizeBlockLen    = "04"
	)

	var sb strings.Builder

	// class descriptor: SC_SERIALIZABLE | SC_WRITE_METHOD with a single int field
	sb.WriteString(streamMagic + streamVersion + tcObject + tcClassDesc)
	sb.WriteString(encodeStr(arrayListName) + arrayListSerial)
	sb.WriteString(orFlags(scSerializable, scWriteMethod))
	sb.WriteString("0001" + hex.EncodeToString([]byte("I")) + encodeStr("size"))
	sb.WriteString(tcEndBlockData + tcNull)

	// the `size` field value, then writeObject's block data and elements
	fmt.Fprintf(&sb, "%08x", uint32(size))
	sb.WriteString(tcBlockData + sizeBlockLen)
	fmt.Fprintf(&sb, "%08x", uint32(size))

	for i := 0; i < size; i++ {
		sb.WriteString(tcString)
		sb.WriteString(encodeStr(fmt.Sprintf("element-%d", i)))
	}

	sb.WriteString(tcEndBlockData)

	return decodeStream(sb.String())
}

// repeatReader endlessly cycles over data so the primitive read benchmarks
// never hit EOF and never pay for reader re-creation.
type repeatReader struct {
	data []byte
	pos  int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		copied := copy(p[n:], r.data[r.pos:])
		n += copied
		r.pos += copied

		if r.pos == len(r.data) {
			r.pos = 0
		}
	}

	return n, nil
}

// newRepeatParser returns a parser over an endless stream of filler bytes.
func newRepeatParser() *SerializedObjectParser {
	return NewSerializedObjectParser(&repeatReader{data: bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 512)})
}

// ---------------------------------- //
// -- Whole Stream Benchmarks ------- //
// ---------------------------------- //

// BenchmarkParseMinimal measures the primary entry point against every
// embedded fixture. These streams are small, so this mostly captures
// per-object overhead rather than throughput.
// partitionFixtures splits the fixtures by whether they currently parse.
// Some exist to pin a rejection -- hashMapObj holds a non-string map key, for
// example -- so running them through a success path benchmark would only
// measure b.Fatalf. Partitioning by probe rather than by a hard coded list
// means this cannot drift as the parser's contract changes.
func partitionFixtures(b *testing.B) (accepted, rejected []string) {
	b.Helper()

	for _, name := range fixtureNames() {
		if _, err := ParseSerializedObjectMinimal(fixture(b, name)); err != nil {
			rejected = append(rejected, name)

			continue
		}

		accepted = append(accepted, name)
	}

	return accepted, rejected
}

func BenchmarkParseMinimal(b *testing.B) {
	accepted, rejected := partitionFixtures(b)

	// Say what was left out, so an excluded fixture is never mistaken for one
	// that was measured. BenchmarkParseRejected covers these.
	if len(rejected) > 0 {
		b.Logf("excluded %d fixture(s) that the parser rejects: %s",
			len(rejected), strings.Join(rejected, ", "))
	}

	for _, name := range accepted {
		data := fixture(b, name)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for i := 0; i < b.N; i++ {
				content, err := ParseSerializedObjectMinimal(data)
				if err != nil {
					b.Fatalf("parse %q: %v", name, err)
				}

				sink = content
			}
		})
	}
}

// BenchmarkParseRejected measures the error path for fixtures the parser
// refuses. Rejection costs real work -- the stream is parsed up to the point
// of failure and the error is wrapped at every frame on the way out -- so it
// is worth tracking separately rather than skipping.
func BenchmarkParseRejected(b *testing.B) {
	_, rejected := partitionFixtures(b)

	if len(rejected) == 0 {
		b.Skip("no fixture is currently rejected")
	}

	for _, name := range rejected {
		data := fixture(b, name)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for i := 0; i < b.N; i++ {
				if _, err := ParseSerializedObjectMinimal(data); err == nil {
					b.Fatalf("fixture %q was expected to be rejected", name)
				}
			}
		})
	}
}

// BenchmarkParseDetailed measures the detailed entry point, which skips the
// jsonFriendly* conversion pass.
func BenchmarkParseDetailed(b *testing.B) {
	for _, name := range []string{"prim", "inherited", "custom", "hashMapStr", "arrayList"} {
		data := fixture(b, name)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for i := 0; i < b.N; i++ {
				content, err := ParseSerializedObject(data)
				if err != nil {
					b.Fatalf("parse %q: %v", name, err)
				}

				sink = content
			}
		})
	}
}

// BenchmarkParseStream measures the io.Reader entry point, isolating the cost
// of the buffered reader from the bytes.Reader fast path.
func BenchmarkParseStream(b *testing.B) {
	for _, name := range []string{"prim", "hashMapStr", "arrayList"} {
		data := fixture(b, name)

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for i := 0; i < b.N; i++ {
				sop := NewSerializedObjectParser(bytes.NewReader(data), SetMaxDataBlockSize(len(data)))

				content, err := sop.ParseSerializedObjectMinimal()
				if err != nil {
					b.Fatalf("parse %q: %v", name, err)
				}

				sink = content
			}
		})
	}
}

// ---------------------------------- //
// -- Array Benchmarks -------------- //
// ---------------------------------- //

// benchmarkArray runs a size sweep over a synthetic array stream builder.
func benchmarkArray(b *testing.B, build func(int) []byte) {
	for _, size := range arraySizes {
		data := build(size)

		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))

			for i := 0; i < b.N; i++ {
				content, err := ParseSerializedObject(data)
				if err != nil {
					b.Fatalf("parse stream of %d elements: %v", size, err)
				}

				sink = content
			}
		})
	}
}

// BenchmarkByteArray covers byte[], the most common shape in real payloads and
// the worst case for per-element boxing.
func BenchmarkByteArray(b *testing.B) {
	benchmarkArray(b, byteArrayStream)
}

// BenchmarkIntArray covers a fixed-width primitive array wider than one byte.
func BenchmarkIntArray(b *testing.B) {
	benchmarkArray(b, intArrayStream)
}

// BenchmarkStringArray covers an object array, which recurses through content.
func BenchmarkStringArray(b *testing.B) {
	benchmarkArray(b, stringArrayStream)
}

// BenchmarkArrayList covers a post-processed collection at realistic sizes.
func BenchmarkArrayList(b *testing.B) {
	benchmarkArray(b, arrayListStream)
}

// ---------------------------------- //
// -- Primitive Read Benchmarks ----- //
// ---------------------------------- //

// BenchmarkReadPrimitive measures the individual stream read helpers, which
// sit on the hottest path in the parser.
func BenchmarkReadPrimitive(b *testing.B) {
	b.Run("uint8", func(b *testing.B) {
		sop := newRepeatParser()
		b.ReportAllocs()
		b.SetBytes(1)

		for i := 0; i < b.N; i++ {
			if _, err := sop.readUInt8(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("uint16", func(b *testing.B) {
		sop := newRepeatParser()
		b.ReportAllocs()
		b.SetBytes(2)

		for i := 0; i < b.N; i++ {
			if _, err := sop.readUInt16(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("int32", func(b *testing.B) {
		sop := newRepeatParser()
		b.ReportAllocs()
		b.SetBytes(4)

		for i := 0; i < b.N; i++ {
			if _, err := sop.readInt32(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("int64", func(b *testing.B) {
		sop := newRepeatParser()
		b.ReportAllocs()
		b.SetBytes(8)

		for i := 0; i < b.N; i++ {
			if _, err := sop.readInt64(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("float64", func(b *testing.B) {
		sop := newRepeatParser()
		b.ReportAllocs()
		b.SetBytes(8)

		for i := 0; i < b.N; i++ {
			if _, err := sop.readFloat64(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkReadString measures variable length string reads across the range
// of lengths that fit in a single utf segment.
func BenchmarkReadString(b *testing.B) {
	for _, size := range []int{8, 256, 8192} {
		payload := bytes.Repeat([]byte("a"), size)
		stream := decodeStream(streamMagic + streamVersion + tcString +
			fmt.Sprintf("%04x", uint16(size)) + hex.EncodeToString(payload))

		b.Run(fmt.Sprintf("len=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				content, err := ParseSerializedObject(stream)
				if err != nil {
					b.Fatalf("parse string of %d: %v", size, err)
				}

				sink = content
			}
		})
	}
}

// ---------------------------------- //
// -- Benchmark Fixture Validation -- //
// ---------------------------------- //

// TestSyntheticStreamsAreValid guards the hand-built benchmark streams. A
// malformed stream that still happens to parse would quietly make the
// benchmarks measure the wrong code path, so assert the decoded shape too.
func TestSyntheticStreamsAreValid(t *testing.T) {
	const size = 8

	t.Run("byteArray", func(t *testing.T) {
		assertArrayOfLen(t, byteArrayStream(size), size)
	})

	t.Run("intArray", func(t *testing.T) {
		content := assertArrayOfLen(t, intArrayStream(size), size)
		if content[size-1] != int32(size-1) {
			t.Fatalf("unexpected last element: got %#v, want %d", content[size-1], size-1)
		}
	})

	t.Run("stringArray", func(t *testing.T) {
		content := assertArrayOfLen(t, stringArrayStream(size), size)
		if content[size-1] != fmt.Sprintf("element-%d", size-1) {
			t.Fatalf("unexpected last element: got %#v", content[size-1])
		}
	})

	// The ArrayList stream must trigger listPostProc, otherwise the benchmark
	// measures raw annotation parsing instead of the collection path.
	t.Run("arrayList", func(t *testing.T) {
		content := assertArrayOfLen(t, arrayListStream(size), size)
		if content[size-1] != fmt.Sprintf("element-%d", size-1) {
			t.Fatalf("unexpected last element: got %#v", content[size-1])
		}
	})
}

// assertArrayOfLen parses a single top level array or collection and asserts
// its element count, returning the elements for further inspection.
func assertArrayOfLen(t *testing.T, stream []byte, want int) []interface{} {
	t.Helper()

	content, err := ParseSerializedObjectMinimal(stream)
	if err != nil {
		t.Fatalf("parse synthetic stream: %v", err)
	}

	if len(content) != 1 {
		t.Fatalf("unexpected top level object count: got %d, want 1", len(content))
	}

	elements, isArray := content[0].([]interface{})
	if !isArray {
		t.Fatalf("unexpected top level type: got %T, want []interface{}", content[0])
	}

	if len(elements) != want {
		t.Fatalf("unexpected element count: got %d, want %d", len(elements), want)
	}

	return elements
}

// TestReadPrimitiveFillerIsEndless verifies the primitive read benchmarks can
// never terminate early on EOF, which would understate their cost.
func TestReadPrimitiveFillerIsEndless(t *testing.T) {
	sop := newRepeatParser()

	for i := 0; i < bufferSize*4; i++ {
		if _, err := sop.readInt64(); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
}
