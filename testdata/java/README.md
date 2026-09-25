# Java serialization fixture generator

`GenerateFixtures.java` recreates every valid Java object stream used by
`deserialize_test.go` and writes the streams as standard Base64 values in a
JSON object.

From the repository root, regenerate the committed fixture file with Java 11
or newer:

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

The generator uses a fresh `ObjectOutputStream` for each fixture. Every stream
contains the same cyclic `Begin` and `End` arrays used by the Go parser tests.
The JSON file stores raw object-stream bytes; compression is intentionally not
part of the fixture format.

The streams are semantically reproducible, but byte-for-byte output from JDK
classes is not guaranteed across Java releases. In particular, exception
internals and collection iteration order are JDK implementation details. Use a
pinned JDK when reviewing golden-file byte changes.
Historical streams whose bytes changed across fixture revisions are kept separately in `testdata/legacy_serialized_objects.json`. They are intentionally not emitted by this generator and are covered by `legacy_deserialize_test.go`.
