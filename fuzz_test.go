//go:build go1.18

package jserial

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// legacyCorpusDir holds the go-fuzz era corpus. Native fuzzing cannot read it
// directly, so the seeds are loaded explicitly below.
const legacyCorpusDir = "fuzzdata/corpus"

// FuzzParse fuzzes the parser and checks three properties on every input:
//
//  1. parsing never panics,
//  2. parsing is deterministic, which catches state leaking between parses
//     through any buffer reused across reads, and
//  3. the []byte and io.Reader entry points agree, given the same maximum
//     data block size.
//
// Note that a crash-only fuzz target cannot observe a parse that returns a
// wrong answer with a nil error, so the properties above are what give this
// target any power beyond finding panics.
func FuzzParse(f *testing.F) {
	for _, name := range fixtureNames() {
		f.Add(objs[name])
	}

	seedFromLegacyCorpus(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		content, err := ParseSerializedObjectMinimal(data)

		// property 2: a second parse of the same bytes must match the first
		repeat, repeatErr := ParseSerializedObjectMinimal(data)
		if (err == nil) != (repeatErr == nil) {
			t.Fatalf("parse not deterministic: first err=%v, second err=%v", err, repeatErr)
		}

		if err == nil && !equalParses(content, repeat) {
			t.Fatalf("parse not deterministic:\nfirst:  %#v\nsecond: %#v", content, repeat)
		}

		// property 3: the streaming entry point must agree with the slice one
		sop := NewSerializedObjectParser(bytes.NewReader(data), SetMaxDataBlockSize(len(data)))

		streamed, streamErr := sop.ParseSerializedObjectMinimal()
		if (err == nil) != (streamErr == nil) {
			t.Fatalf("entry points disagree: []byte err=%v, io.Reader err=%v", err, streamErr)
		}

		if err == nil && !equalParses(content, streamed) {
			t.Fatalf("entry points disagree:\n[]byte:    %#v\nio.Reader: %#v", content, streamed)
		}
	})
}

// nanSentinel stands in for a NaN float during comparison. A distinct type is
// used rather than a string so that it cannot collide with parsed data.
type nanSentinel struct{}

// normalizeNaN replaces NaN floats with a sentinel so that two parses of the
// same bytes compare equal. reflect.DeepEqual reports NaN as unequal to
// itself, and Java streams can legitimately carry NaN in a float or double.
func normalizeNaN(value interface{}) interface{} {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) {
			return nanSentinel{}
		}

	case float32:
		if math.IsNaN(float64(typed)) {
			return nanSentinel{}
		}

	case []interface{}:
		normalized := make([]interface{}, len(typed))
		for i, element := range typed {
			normalized[i] = normalizeNaN(element)
		}

		return normalized

	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, element := range typed {
			normalized[key] = normalizeNaN(element)
		}

		return normalized
	}

	return value
}

// equalParses compares two parse results, tolerating NaN.
func equalParses(a, b []interface{}) bool {
	return reflect.DeepEqual(normalizeNaN(a), normalizeNaN(b))
}

// seedFromLegacyCorpus adds the committed go-fuzz corpus as native seeds.
func seedFromLegacyCorpus(f *testing.F) {
	f.Helper()

	entries, err := os.ReadDir(legacyCorpusDir)
	if err != nil {
		// the corpus is an optimization, not a requirement
		f.Logf("skipping legacy corpus %q: %v", legacyCorpusDir, err)

		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(legacyCorpusDir, entry.Name()))
		if err != nil {
			f.Fatalf("read legacy corpus seed %q: %v", entry.Name(), err)
		}

		f.Add(data)
	}
}
