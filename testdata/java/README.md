# Java serialization fixture generator

`GenerateFixtures.java` recreates the Java object streams used by the Go tests
and benchmarks. It writes three files:

- `serialized_objects.json` contains the sentinel-wrapped functional fixtures;
- `oracle_fixtures.json` contains bare streams and Java-derived leaf values;
- `benchmark_fixtures.json` contains bare, scaled string-list map streams.

All streams are stored as standard Base64 values in JSON objects. From the
repository root, regenerate the committed fixture files with Java 11 or newer:

```sh
java testdata/java/GenerateFixtures.java testdata/serialized_objects.json
```

The source is Java 8 compatible. To use JDK 8, compile it into a temporary
build directory and run it from there:

```sh
build_dir="$(mktemp -d)"
javac -d "$build_dir" testdata/java/GenerateFixtures.java
java -cp "$build_dir" GenerateFixtures testdata/serialized_objects.json
rm -rf "$build_dir"
```

The generator uses a fresh `ObjectOutputStream` for each fixture. Functional
fixtures contain the cyclic `Begin` and `End` arrays used by the original Go
parser tests. Oracle and benchmark fixtures are bare single-object streams, so
they match their Java ground truth and measured parser workload without
sentinel overhead. The JSON files store raw object-stream bytes; compression
is intentionally not part of the fixture format.

The streams are semantically reproducible, but byte-for-byte output from JDK
classes is not guaranteed across Java releases. In particular, exception
internals and collection iteration order are JDK implementation details. Use a
pinned JDK when reviewing golden-file byte changes.
Historical streams whose bytes changed across fixture revisions are kept separately in `testdata/legacy_serialized_objects.json`. They are intentionally not emitted by this generator and are covered by `legacy_deserialize_test.go`.
