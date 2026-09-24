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
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.Date;
import java.util.EnumMap;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Hashtable;
import java.util.LinkedHashMap;
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
