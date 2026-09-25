package jserial

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Oracle tests compare jserial against Java itself. GenerateFixtures.java
// serializes a value, reads it back through ObjectInputStream, and records the
// leaf values Java recovered. These tests assert that jserial recovers the
// same multiset of leaves from the same bytes.
//
// Only leaf values are compared, never structure: jserial renders an object
// and a map identically as a string keyed map, so field names and map keys
// cannot be told apart here. Map keys are therefore skipped on both sides and
// only values count. That makes these tests a check on value fidelity, which
// is where a port is most likely to be silently wrong.

//go:embed testdata/oracle_fixtures.json
var oracleFixturesJSON []byte

type oracleFixture struct {
	Bytes  string   `json:"bytes"`
	Leaves []string `json:"leaves"`
}

// knownDivergence describes a fixture where jserial does not agree with Java.
//
// The two kinds are kept distinct on purpose. A fixture with wantError is
// expected to be *rejected*, and must be rejected for that specific reason;
// anything else -- a different error, or a successful parse -- fails the test.
// A fixture without wantError is expected to parse successfully and merely
// return the wrong values, so a parse error there is a regression rather than
// an accepted divergence.
type knownDivergence struct {
	// reason explains why jserial and Java disagree.
	reason string

	// wantError, when set, requires the parse to fail with an error containing
	// this substring. When empty, the parse must succeed and the recovered
	// leaves must differ from Java's.
	wantError string
}

// knownBroken pins every fixture where jserial disagrees with Java, so that
// fixing a bug fails this test and forces its entry to be removed.
var knownBroken = map[string]knownDivergence{
	"strNul": {
		reason: "readString does not decode Java modified UTF-8, so NUL arrives as the raw bytes C0 80",
	},
	"strEmoji": {
		reason: "readString does not decode CESU-8, so a supplementary character arrives as two raw 3 byte surrogates",
	},
	"strEmojiMixed": {
		reason: "same CESU-8 decoding gap as strEmoji",
	},
	"strLoneHighSurrogate": {
		reason: "same CESU-8 decoding gap as strEmoji",
	},
	"strArrayUnicode": {
		reason: "array member hits the same CESU-8 decoding gap as strEmoji",
	},
	"charSurrogate": {
		reason: "a char holding half a surrogate pair is converted with string(rune(...)), which yields U+FFFD",
	},
	"dateBeyond2262": {
		reason: "datePostProc multiplies milliseconds by time.Millisecond, overflowing int64 past 2262",
	},
	"cycle": {
		reason: "parseObject fills its object handle only after reading fields, so a back reference into the object being built resolves to nil",
	},
	"hashMapIntKeys": {
		reason:    "a Map<Integer,...> cannot be represented, so it is rejected rather than silently emptied",
		wantError: "unsupported map key type java.lang.Integer",
	},
	"hashSetMixed": {
		reason:    "a Set with mixed member types cannot be represented, so it is rejected rather than silently shrunk",
		wantError: "unsupported set member type java.lang.Integer",
	},
}

func loadOracleFixtures(t *testing.T) map[string]oracleFixture {
	t.Helper()

	var fixtures map[string]oracleFixture
	if err := json.Unmarshal(oracleFixturesJSON, &fixtures); err != nil {
		t.Fatalf("unmarshal oracle fixtures: %v", err)
	}

	if len(fixtures) == 0 {
		t.Fatal("oracle fixtures are empty")
	}

	return fixtures
}

func TestOracleLeafValues(t *testing.T) {
	fixtures := loadOracleFixtures(t)

	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		fixture := fixtures[name]

		t.Run(name, func(t *testing.T) {
			stream, err := base64.StdEncoding.Strict().DecodeString(fixture.Bytes)
			if err != nil {
				t.Fatalf("decode fixture bytes: %v", err)
			}

			want := fixture.Leaves
			divergence, known := knownBroken[name]

			content, err := ParseSerializedObjectMinimal(stream)

			// A fixture pinned to a specific rejection must be rejected for
			// exactly that reason. An unrelated error is a regression, not a
			// pass.
			if known && divergence.wantError != "" {
				if err == nil {
					t.Fatalf("fixture is pinned to fail with %q but parsed successfully.\n"+
						"Remove or update the knownBroken entry. Recorded reason: %s",
						divergence.wantError, divergence.reason)
				}

				if !strings.Contains(err.Error(), divergence.wantError) {
					t.Fatalf("fixture is pinned to fail with %q but failed differently: %v",
						divergence.wantError, err)
				}

				t.Logf("known rejection: %s\n  java:    %s\n  jserial: %v",
					divergence.reason, strings.Join(want, " "), err)

				return
			}

			// Every other fixture, diverging or not, must parse. A known
			// value divergence that starts erroring is a regression.
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			got := collectLeaves(content)

			matches := reflect.DeepEqual(got, want)

			if known {
				// Pinned: a known bug must still reproduce, so that fixing it
				// forces this entry to be removed from knownBroken.
				if matches {
					t.Fatalf("fixture is listed in knownBroken but now matches Java.\n"+
						"Remove the entry. Recorded reason: %s", divergence.reason)
				}

				t.Logf("known divergence from Java: %s\n  java:    %s\n  jserial: %s",
					divergence.reason, strings.Join(want, " "), strings.Join(got, " "))

				return
			}

			if !matches {
				t.Fatalf("leaf values differ from Java\n  java:    %s\n  jserial: %s",
					strings.Join(want, " "), strings.Join(got, " "))
			}
		})
	}
}

// TestOracleReportDivergences prints every fixture and whether it matches
// Java. Run with ORACLE_REPORT=1 to see the full table, which is the quickest
// way to review what the oracle currently covers.
func TestOracleReportDivergences(t *testing.T) {
	if os.Getenv("ORACLE_REPORT") == "" {
		t.Skip("set ORACLE_REPORT=1 to print the oracle comparison table")
	}

	fixtures := loadOracleFixtures(t)

	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		fixture := fixtures[name]
		stream, _ := base64.StdEncoding.Strict().DecodeString(fixture.Bytes)

		status := "MATCH"

		content, err := ParseSerializedObjectMinimal(stream)
		got := []string(nil)

		switch {
		case err != nil:
			status = "PARSE ERROR"
		default:
			got = collectLeaves(content)
			if !reflect.DeepEqual(got, fixture.Leaves) {
				status = "DIFFERS"
			}
		}

		fmt.Printf("%-11s %-22s java=%v\n", status, name, fixture.Leaves)
		if status != "MATCH" {
			fmt.Printf("%-11s %-22s go  =%v\n", "", "", got)
		}
	}
}

// collectLeaves renders every leaf value in a parsed result using the same
// canonical encoding as the Java side, sorted for order independent
// comparison.
func collectLeaves(content []interface{}) []string {
	leaves := make([]string, 0)
	for _, value := range content {
		leaves = appendLeaves(leaves, value, make(map[interface{}]bool))
	}

	sort.Strings(leaves)

	return leaves
}

//nolint:gocyclo // a flat type switch over every representable leaf type
func appendLeaves(leaves []string, value interface{}, path map[interface{}]bool) []string {
	switch typed := value.(type) {
	case nil:
		return append(leaves, "null:")

	case string:
		return append(leaves, "string:"+describeCodePoints(typed))

	case bool:
		return append(leaves, fmt.Sprintf("boolean:%t", typed))

	case int8:
		return append(leaves, fmt.Sprintf("byte:%d", typed))

	case int16:
		return append(leaves, fmt.Sprintf("short:%d", typed))

	case int32:
		return append(leaves, fmt.Sprintf("int:%d", typed))

	case int64:
		return append(leaves, fmt.Sprintf("long:%d", typed))

	case float32:
		return append(leaves, fmt.Sprintf("float:0x%08x", math.Float32bits(typed)))

	case float64:
		return append(leaves, fmt.Sprintf("double:0x%016x", math.Float64bits(typed)))

	case time.Time:
		// UnixMilli, not UnixNano/1e6: UnixNano itself overflows outside
		// 1678-2262, which would make this encoder reproduce the very
		// overflow the dateBeyond2262 fixture exists to detect.
		return append(leaves, fmt.Sprintf("date:%d", typed.UnixMilli()))

	case []byte:
		for _, b := range typed {
			leaves = append(leaves, fmt.Sprintf("byte:%d", int8(b)))
		}

		return leaves

	case []interface{}:
		for _, element := range typed {
			leaves = appendLeaves(leaves, element, path)
		}

		return leaves

	case map[string]bool:
		// hashSetPostProc renders a set with its members as keys
		for member := range typed {
			leaves = append(leaves, "string:"+describeCodePoints(member))
		}

		return leaves

	case map[string]interface{}:
		// Values only. Keys are skipped because a field name and a map key are
		// indistinguishable in this representation.
		for _, element := range typed {
			leaves = appendLeaves(leaves, element, path)
		}

		return leaves

	default:
		return append(leaves, fmt.Sprintf("unhandled:%T", value))
	}
}

// describeCodePoints renders a string as its code points so that an encoding
// bug cannot hide behind a byte comparison.
func describeCodePoints(value string) string {
	points := make([]string, 0, len(value))
	for _, point := range value {
		points = append(points, fmt.Sprintf("U+%04X", point))
	}

	return strings.Join(points, ",")
}
