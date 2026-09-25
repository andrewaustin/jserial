import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.EOFException;
import java.io.Externalizable;
import java.io.IOException;
import java.io.ObjectInput;
import java.io.ObjectInputStream;
import java.io.ObjectOutput;
import java.io.ObjectOutputStream;
import java.io.Serializable;
import java.lang.reflect.Array;
import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.Collections;
import java.util.Date;
import java.util.EnumMap;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Hashtable;
import java.util.IdentityHashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/** Generates the Java object-stream fixtures consumed by the Go tests. */
public final class GenerateFixtures {
    private static final int LONG_STRING_LENGTH = 131_072;
    private static final long FIXTURE_DATE_MILLIS = 403_879_620_000L;

    private GenerateFixtures() {
    }

    public static void main(String[] args) throws Exception {
        if (args.length > 1) {
            throw new IllegalArgumentException(
                    "usage: GenerateFixtures [output.json]");
        }

        Path output = Paths.get(args.length == 1
                ? args[0]
                : "testdata/serialized_objects.json");
        Map<String, byte[]> fixtures = generateFixtures();
        verifyRoundTrips(fixtures);
        writeJson(output, fixtures);

        System.out.printf("Wrote %d fixtures to %s%n",
                fixtures.size(), output.toAbsolutePath());

        Path oracleOutput = output.resolveSibling("oracle_fixtures.json");
        Map<String, OracleFixture> oracle = generateOracleFixtures();
        writeOracleJson(oracleOutput, oracle);

        System.out.printf("Wrote %d oracle fixtures to %s%n",
                oracle.size(), oracleOutput.toAbsolutePath());
    }

    static byte[] shortPattern() {
        return new byte[] {
            (byte) 0xb5, (byte) 0xeb, 0x2d, 0x00,
            (byte) 0xb5, (byte) 0xeb, 0x2d, 0x00,
            (byte) 0xb5, (byte) 0xeb, 0x2d
        };
    }

    private static Map<String, byte[]> generateFixtures() throws IOException {
        Map<String, byte[]> fixtures = new LinkedHashMap<String, byte[]>();

        add(fixtures, "canary", out -> { });
        add(fixtures, "string", out -> out.writeObject("sometext"));
        add(fixtures, "longStr", out -> out.writeObject(repeatedX()));
        add(fixtures, "null", out -> out.writeObject(null));

        add(fixtures, "dupe", out -> {
            BaseClassWithField value = new BaseClassWithField();
            value.foo = 123;
            out.writeObject(value);
            out.writeObject("delim");
            out.writeObject(value);
        });

        add(fixtures, "prim", out -> {
            PrimitiveFields value = new PrimitiveFields();
            value.bo = true;
            value.by = (byte) -21;
            value.c = '\u1234';
            value.d = 12.34d;
            value.f = 76.5f;
            value.i = -123;
            value.l = -789L;
            value.s = (short) -456;
            out.writeObject(value);
        });

        add(fixtures, "boxedPrim", out -> {
            out.writeObject(Integer.valueOf(-123));
            out.writeObject(Short.valueOf((short) -456));
            out.writeObject(Long.valueOf(-789L));
            out.writeObject(Byte.valueOf((byte) -21));
            out.writeObject(Double.valueOf(12.34d));
            out.writeObject(Float.valueOf(76.5f));
            out.writeObject(Boolean.TRUE);
            out.writeObject(Character.valueOf('\u1234'));
        });

        add(fixtures, "inherited", out -> {
            DerivedClassWithAnotherField value =
                    new DerivedClassWithAnotherField();
            value.foo = 123;
            value.bar = 234;
            out.writeObject(value);
        });

        add(fixtures, "dupeField", out -> {
            DerivedClassWithSameField value = new DerivedClassWithSameField();
            ((BaseClassWithField) value).foo = 123;
            value.foo = 345;
            out.writeObject(value);
        });

        add(fixtures, "primArray",
                out -> out.writeObject(new int[] {12, 34, 56}));
        add(fixtures, "nestedArr", out -> out.writeObject(
                new String[][] {{"a", "b"}, {"c"}}));

        add(fixtures, "arrFields", out -> {
            ArrayFields value = new ArrayFields();
            value.ia = new int[] {12, 34, 56};
            value.iaa = new int[][] {{11, 12}, {21, 22, 23}};
            value.sa = new String[] {"foo", "bar"};
            out.writeObject(value);
        });

        add(fixtures, "enum", out -> {
            out.writeObject(SomeEnum.ONE);
            out.writeObject(SomeEnum.THREE);
            out.writeObject(SomeEnum.THREE);
        });

        add(fixtures, "exception", out -> {
            RuntimeException value = new RuntimeException("Kaboom");
            value.setStackTrace(new StackTraceElement[] {
                new StackTraceElement("Cls", "met", "Cls.java", 123)
            });
            out.writeObject(value);
        });

        add(fixtures, "custom", out -> out.writeObject(new CustomFormat()));
        add(fixtures, "extern",
                out -> out.writeObject(new External(shortPattern())));

        add(fixtures, "longExtern", out -> {
            byte[] data = new byte[512];
            for (int i = 0; i < data.length; i++) {
                data[i] = (byte) i;
            }
            out.writeObject(new External(data));
        });

        add(fixtures, "hashMapStr", out -> {
            Map<String, Object> value = new HashMap<String, Object>();
            value.put("foo", Integer.valueOf(123));
            value.put("bar", "baz");
            out.writeObject(value);
        });

        add(fixtures, "hashMapObj", out -> {
            Integer key = Integer.valueOf(123);
            Map<Object, Object> value = new HashMap<Object, Object>();
            value.put("baz", "bar");
            value.put(key, "foo");
            out.writeObject(value);
            out.writeObject(key);
        });

        add(fixtures, "hashMapEmpty",
                out -> out.writeObject(new HashMap<Object, Object>()));

        add(fixtures, "hashTblStr", out -> {
            Hashtable<String, Object> value =
                    new Hashtable<String, Object>();
            value.put("foo", Integer.valueOf(123));
            value.put("bar", "baz");
            out.writeObject(value);
        });

        add(fixtures, "enumMap", out -> {
            EnumMap<SomeEnum, Object> value =
                    new EnumMap<SomeEnum, Object>(SomeEnum.class);
            value.put(SomeEnum.ONE, Integer.valueOf(123));
            value.put(SomeEnum.THREE, "baz");
            out.writeObject(value);
            out.writeObject(SomeEnum.ONE);
            out.writeObject(SomeEnum.THREE);
        });

        add(fixtures, "arrayList", out -> {
            ArrayList<Object> value = new ArrayList<Object>();
            value.add("foo");
            value.add(Integer.valueOf(123));
            out.writeObject(value);
        });

        add(fixtures, "arrayDeque", out -> {
            ArrayDeque<Object> value = new ArrayDeque<Object>();
            value.add("foo");
            value.add(Integer.valueOf(123));
            out.writeObject(value);
        });

        add(fixtures, "hashSet", out -> {
            HashSet<String> value = new HashSet<String>();
            value.add("foo");
            value.add("bar");
            out.writeObject(value);
        });

        add(fixtures, "date",
                out -> out.writeObject(new Date(FIXTURE_DATE_MILLIS)));

        return fixtures;
    }

    private static void add(Map<String, byte[]> fixtures, String name,
            FixtureWriter writer) throws IOException {
        fixtures.put(name, serialize(writer));
    }

    private static byte[] serialize(FixtureWriter writer) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        try (ObjectOutputStream out = new ObjectOutputStream(bytes)) {
            Object[] begin = new Object[2];
            begin[0] = "Begin";
            begin[1] = begin;
            out.writeObject(begin);

            writer.write(out);

            Object[] end = new Object[2];
            end[0] = end;
            end[1] = "End";
            out.writeObject(end);
        }
        return bytes.toByteArray();
    }

    private static String repeatedX() {
        char[] chars = new char[LONG_STRING_LENGTH];
        Arrays.fill(chars, 'x');
        return new String(chars);
    }

    private static void writeJson(Path output, Map<String, byte[]> fixtures)
            throws IOException {
        Path absoluteOutput = output.toAbsolutePath();
        Path parent = absoluteOutput.getParent();
        if (parent != null) {
            Files.createDirectories(parent);
        }

        StringBuilder json = new StringBuilder();
        json.append("{\n");
        int index = 0;
        for (Map.Entry<String, byte[]> fixture : fixtures.entrySet()) {
            json.append("  \"")
                    .append(fixture.getKey())
                    .append("\": \"")
                    .append(Base64.getEncoder().encodeToString(
                            fixture.getValue()))
                    .append('"');
            if (++index < fixtures.size()) {
                json.append(',');
            }
            json.append('\n');
        }
        json.append("}\n");

        Files.write(absoluteOutput,
                json.toString().getBytes(StandardCharsets.UTF_8));
    }


    // ------------------------------------------------------------------ //
    // -- Oracle fixtures ----------------------------------------------- //
    // ------------------------------------------------------------------ //

    /**
     * Pairs a serialized stream with the leaf values Java itself recovers
     * from that stream. The Go side asserts it recovers the same multiset of
     * leaves, which catches values that decode to the wrong thing without any
     * error being reported.
     *
     * <p>Leaves are compared as an order independent multiset rather than by
     * path, so this deliberately says nothing about structure. It checks value
     * fidelity only, which is where a port is most likely to go wrong.
     */
    private static final class OracleFixture {
        private final byte[] bytes;
        private final List<String> leaves;

        OracleFixture(byte[] bytes, List<String> leaves) {
            this.bytes = bytes;
            this.leaves = leaves;
        }
    }

    /** Builds a string holding the single given code point. */
    private static String codePoint(int value) {
        return new String(new int[] {value}, 0, 1);
    }

    private static Map<String, Object> oracleValues() {
        Map<String, Object> values = new LinkedHashMap<String, Object>();

        // Java writes strings as modified UTF-8: NUL becomes two bytes and a
        // supplementary character becomes a CESU-8 surrogate pair, so neither
        // matches standard UTF-8 on the wire.
        values.put("strAscii", "hello");
        values.put("strLatin1", "caf\u00e9");
        values.put("strBmp3Byte", "\u1234");
        values.put("strEmpty", "");
        values.put("strNul", "a" + (char) 0 + "b");
        values.put("strEmoji", codePoint(0x1F600));
        values.put("strEmojiMixed", "hi " + codePoint(0x1F600) + " there");
        values.put("strLoneHighSurrogate", "x" + (char) 0xD83D + "y");
        values.put("strArrayUnicode",
                new String[] {codePoint(0x1F600), "plain", ""});

        // char is a UTF-16 code unit, so it can hold half a surrogate pair.
        values.put("charBmp", Character.valueOf('\u1234'));
        values.put("charSurrogate", Character.valueOf((char) 0xD83D));
        values.put("charNul", Character.valueOf((char) 0));

        // Dates past 2262 overflow a nanosecond representation.
        values.put("dateEpoch", new Date(0L));
        values.put("datePre1970", new Date(-86_400_000L));
        values.put("dateBeyond2262", new Date(9_300_000_000_000L));

        // Float and double edge cases, compared as raw bits.
        values.put("doubleNaN", Double.valueOf(Double.NaN));
        values.put("doublePosInf", Double.valueOf(Double.POSITIVE_INFINITY));
        values.put("doubleNegZero", Double.valueOf(-0.0d));
        values.put("floatNaN", Float.valueOf(Float.NaN));
        values.put("intMin", Integer.valueOf(Integer.MIN_VALUE));
        values.put("longMin", Long.valueOf(Long.MIN_VALUE));

        // Map keys that are not strings, and a set of mixed member types.
        Map<Object, Object> intKeys = new HashMap<Object, Object>();
        intKeys.put(Integer.valueOf(1), "one");
        intKeys.put(Integer.valueOf(2), "two");
        values.put("hashMapIntKeys", intKeys);

        HashSet<Object> mixedSet = new HashSet<Object>();
        mixedSet.add("foo");
        mixedSet.add(Integer.valueOf(123));
        values.put("hashSetMixed", mixedSet);

        // A reference cycle. Java restores the identity cycle, so a parser
        // that registers object handles too late loses the back reference.
        OracleNode first = new OracleNode("a");
        OracleNode second = new OracleNode("b");
        first.next = second;
        second.next = first;
        values.put("cycle", first);

        // The same object referenced twice, which is a shared substructure
        // rather than a cycle.
        OracleNode shared = new OracleNode("shared");
        values.put("sharedRef", new OracleNode[] {shared, shared});

        return values;
    }

    private static Map<String, OracleFixture> generateOracleFixtures()
            throws IOException, ClassNotFoundException {
        Map<String, OracleFixture> fixtures =
                new LinkedHashMap<String, OracleFixture>();

        for (Map.Entry<String, Object> entry : oracleValues().entrySet()) {
            final Object value = entry.getValue();
            byte[] bytes = serializeBare(new FixtureWriter() {
                @Override
                public void write(ObjectOutputStream out) throws IOException {
                    out.writeObject(value);
                }
            });

            // Read the stream back through Java so the expected leaves come
            // from ObjectInputStream, not from the in memory value.
            Object restored;
            try (ObjectInputStream in = new ObjectInputStream(
                    new ByteArrayInputStream(bytes))) {
                restored = in.readObject();
            }

            fixtures.put(entry.getKey(),
                    new OracleFixture(bytes, oracleLeaves(restored)));
        }

        return fixtures;
    }

    /** Serializes a single object with no surrounding sentinel objects. */
    private static byte[] serializeBare(FixtureWriter writer)
            throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        try (ObjectOutputStream out = new ObjectOutputStream(bytes)) {
            writer.write(out);
        }
        return bytes.toByteArray();
    }

    /** Returns every leaf value reachable from root, sorted. */
    private static List<String> oracleLeaves(Object root) {
        List<String> leaves = new ArrayList<String>();
        collectLeaves(root, leaves, new IdentityHashMap<Object, Boolean>());
        Collections.sort(leaves);

        return leaves;
    }

    private static void collectLeaves(Object value, List<String> leaves,
            Map<Object, Boolean> seen) {
        if (value == null) {
            leaves.add("null:");

            return;
        }

        if (value instanceof String) {
            leaves.add("string:" + describeCodePoints((String) value));

            return;
        }

        if (value instanceof Character) {
            // jserial represents a Java char as a one character string
            leaves.add("string:" + describeCodePoints(
                    String.valueOf(((Character) value).charValue())));

            return;
        }

        if (value instanceof Boolean) {
            leaves.add("boolean:" + value);

            return;
        }

        if (value instanceof Byte) {
            leaves.add("byte:" + value);

            return;
        }

        if (value instanceof Short) {
            leaves.add("short:" + value);

            return;
        }

        if (value instanceof Integer) {
            leaves.add("int:" + value);

            return;
        }

        if (value instanceof Long) {
            leaves.add("long:" + value);

            return;
        }

        if (value instanceof Float) {
            leaves.add(String.format("float:0x%08x",
                    Integer.valueOf(Float.floatToRawIntBits(
                            ((Float) value).floatValue()))));

            return;
        }

        if (value instanceof Double) {
            leaves.add(String.format("double:0x%016x",
                    Long.valueOf(Double.doubleToRawLongBits(
                            ((Double) value).doubleValue()))));

            return;
        }

        if (value instanceof Date) {
            leaves.add("date:" + ((Date) value).getTime());

            return;
        }

        // `seen` holds the current path rather than every object visited, so
        // shared acyclic substructure is expanded on each visit the same way
        // jserial expands a repeated reference, while a true cycle is cut.
        if (seen.put(value, Boolean.TRUE) != null) {
            return;
        }

        try {
            // Map keys are skipped: jserial renders both an object and a map
            // as a string keyed map, so field names and map keys cannot be
            // told apart on the Go side. Only values are compared.
            if (value instanceof Map) {
                for (Object entry : ((Map<?, ?>) value).entrySet()) {
                    collectLeaves(((Map.Entry<?, ?>) entry).getValue(),
                            leaves, seen);
                }

                return;
            }

            if (value instanceof Iterable) {
                for (Object element : (Iterable<?>) value) {
                    collectLeaves(element, leaves, seen);
                }

                return;
            }

            if (value.getClass().isArray()) {
                int length = Array.getLength(value);
                for (int i = 0; i < length; i++) {
                    collectLeaves(Array.get(value, i), leaves, seen);
                }

                return;
            }

            collectFieldLeaves(value, leaves, seen);
        } finally {
            seen.remove(value);
        }
    }

    private static void collectFieldLeaves(Object value, List<String> leaves,
            Map<Object, Boolean> seen) {
        for (Class<?> type = value.getClass();
                type != null && type != Object.class;
                type = type.getSuperclass()) {
            for (Field field : type.getDeclaredFields()) {
                int modifiers = field.getModifiers();
                if (Modifier.isStatic(modifiers)
                        || Modifier.isTransient(modifiers)) {
                    continue;
                }

                field.setAccessible(true);
                try {
                    collectLeaves(field.get(value), leaves, seen);
                } catch (IllegalAccessException error) {
                    throw new IllegalStateException(
                            "unable to read field " + field, error);
                }
            }
        }
    }

    /** Renders a string as its code points so encoding bugs cannot hide. */
    private static String describeCodePoints(String value) {
        StringBuilder rendered = new StringBuilder();
        for (int i = 0; i < value.length(); ) {
            int point = value.codePointAt(i);
            if (rendered.length() > 0) {
                rendered.append(',');
            }
            rendered.append(String.format("U+%04X", Integer.valueOf(point)));
            i += Character.charCount(point);
        }

        return rendered.toString();
    }

    private static void writeOracleJson(Path output,
            Map<String, OracleFixture> fixtures) throws IOException {
        Path absoluteOutput = output.toAbsolutePath();
        Path parent = absoluteOutput.getParent();
        if (parent != null) {
            Files.createDirectories(parent);
        }

        StringBuilder json = new StringBuilder();
        json.append("{\n");
        int index = 0;
        for (Map.Entry<String, OracleFixture> fixture : fixtures.entrySet()) {
            OracleFixture value = fixture.getValue();
            json.append("  \"").append(fixture.getKey()).append("\": {\n");
            json.append("    \"bytes\": \"")
                    .append(Base64.getEncoder().encodeToString(value.bytes))
                    .append("\",\n");
            json.append("    \"leaves\": [");
            for (int leaf = 0; leaf < value.leaves.size(); leaf++) {
                if (leaf > 0) {
                    json.append(", ");
                }
                json.append('"').append(value.leaves.get(leaf)).append('"');
            }
            json.append("]\n  }");
            if (++index < fixtures.size()) {
                json.append(',');
            }
            json.append('\n');
        }
        json.append("}\n");

        Files.write(absoluteOutput,
                json.toString().getBytes(StandardCharsets.UTF_8));
    }

    private static void verifyRoundTrips(Map<String, byte[]> fixtures)
            throws IOException {
        for (Map.Entry<String, byte[]> fixture : fixtures.entrySet()) {
            try (ObjectInputStream in = new ObjectInputStream(
                    new ByteArrayInputStream(fixture.getValue()))) {
                while (true) {
                    in.readObject();
                }
            } catch (EOFException expected) {
                // A complete object stream terminates at EOF.
            } catch (IOException | ClassNotFoundException error) {
                throw new IOException("fixture failed Java round trip: "
                        + fixture.getKey(), error);
            }
        }
    }

    @FunctionalInterface
    private interface FixtureWriter {
        void write(ObjectOutputStream out) throws IOException;
    }
}

class BaseClassWithField implements Serializable {
    private static final long serialVersionUID = 0x1234L;

    int foo;
}

class DerivedClassWithAnotherField extends BaseClassWithField {
    private static final long serialVersionUID = 0x2345L;

    int bar;
}

class DerivedClassWithSameField extends BaseClassWithField {
    private static final long serialVersionUID = 0x3456L;

    int foo;
}

class PrimitiveFields implements Serializable {
    private static final long serialVersionUID = 0x123456789ABCL;

    boolean bo;
    byte by;
    char c;
    double d;
    float f;
    int i;
    long l;
    short s;
}

class ArrayFields implements Serializable {
    private static final long serialVersionUID = 1L;

    int[] ia;
    int[][] iaa;
    String[] sa;
}

enum SomeEnum {
    ONE,
    TWO,
    THREE
}

class CustomFormat implements Serializable {
    private static final long serialVersionUID = 1L;

    int foo = 12345;

    private void writeObject(ObjectOutputStream out) throws IOException {
        out.defaultWriteObject();
        out.write(GenerateFixtures.shortPattern());
        out.writeObject("and more");
    }
}

class External implements Externalizable {
    private static final long serialVersionUID = 0xf0df60b4d1321d11L;

    private byte[] data;

    public External() {
        data = new byte[0];
    }

    External(byte[] data) {
        this.data = data.clone();
    }

    @Override
    public void writeExternal(ObjectOutput out) throws IOException {
        out.writeInt(data.length);
        out.write(data);
        out.writeObject("and more");
    }

    @Override
    public void readExternal(ObjectInput in)
            throws IOException, ClassNotFoundException {
        int length = in.readInt();
        data = new byte[length];
        in.readFully(data);
        in.readObject();
    }
}

class OracleNode implements Serializable {
    private static final long serialVersionUID = 0xABCDEFL;
    String name;
    OracleNode next;

    OracleNode(String name) {
        this.name = name;
    }
}
