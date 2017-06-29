package resp

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

var decodeErrCases = []struct {
	enc []byte
	val interface{}
	err error
}{
	{[]byte("+ceci n'est pas un string"), nil, io.EOF},
	{[]byte("+"), nil, io.EOF},
	{[]byte("+a\rZ"), nil, ErrMissingCRLF},
	{[]byte("-ceci n'est pas un string"), nil, io.EOF},
	{[]byte("-"), nil, io.EOF},
	{[]byte(":123\n"), int64(0), ErrMissingCRLF},
	{[]byte(":123\rZ"), int64(0), ErrMissingCRLF},
	{[]byte(":123456789012345678901\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":-12345678901234567890\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":123a\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":123"), int64(0), io.EOF},
	{[]byte(":-1-3\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":12-3\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":123-\r\n"), int64(0), ErrInvalidInteger},
	{[]byte(":"), int64(0), io.EOF},
	{[]byte("$"), nil, io.EOF},
	{[]byte("$6\r\nc\r\n"), nil, io.ErrUnexpectedEOF},
	{[]byte("$6\r\nabc\r\n"), nil, io.ErrUnexpectedEOF},
	{[]byte("$6\nabc\r\n"), nil, ErrMissingCRLF},
	{[]byte("$4\r\nabc\r\n"), nil, io.ErrUnexpectedEOF},
	{[]byte("$-3\r\n"), nil, ErrInvalidBulkString},
	{[]byte("$70\r\n"), nil, ErrInvalidBulkString},
	{[]byte("$3\r\nabcZ\n"), nil, ErrMissingCRLF},
	{[]byte("$3\r\nabc\rZ"), nil, ErrMissingCRLF},
	{[]byte("*1\n:10\r\n"), Array(nil), ErrMissingCRLF},
	{[]byte("*-3\r\n"), Array(nil), ErrInvalidArray},
	{[]byte(":\r\n"), int64(0), nil},
	{[]byte("$\r\n\r\n"), "", nil},
	{[]byte("!\r\n"), nil, ErrInvalidPrefix},
	{[]byte("*1\r\n:1-\r\n"), Array(nil), ErrInvalidInteger},
}

var decodeValidCases = []struct {
	enc []byte
	val interface{}
	err error
}{
	{[]byte{'+', '\r', '\n'}, "", nil},
	{[]byte{'+', 'a', '\r', '\n'}, "a", nil},
	{[]byte{'+', 'O', 'K', '\r', '\n'}, "OK", nil},
	{[]byte("+ceci n'est pas un string\r\n"), "ceci n'est pas un string", nil},
	{[]byte{'-', '\r', '\n'}, "", nil},
	{[]byte{'-', 'a', '\r', '\n'}, "a", nil},
	{[]byte{'-', 'K', 'O', '\r', '\n'}, "KO", nil},
	{[]byte("-ceci n'est pas un string\r\n"), "ceci n'est pas un string", nil},
	{[]byte(":1\r\n"), int64(1), nil},
	{[]byte(":123\r\n"), int64(123), nil},
	{[]byte(":-123\r\n"), int64(-123), nil},
	{[]byte(":1234567890123456789\r\n"), int64(1234567890123456789), nil},
	{[]byte(":-123456789012345678\r\n"), int64(-123456789012345678), nil},
	{[]byte("$0\r\n\r\n"), "", nil},
	{[]byte("$24\r\nceci n'est pas un string\r\n"), "ceci n'est pas un string", nil},
	{[]byte("$51\r\nceci n'est pas un string\r\navec\rdes\nsauts\r\nde\x00ligne.\r\n"), "ceci n'est pas un string\r\navec\rdes\nsauts\r\nde\x00ligne.", nil},
	{[]byte("$-1\r\n"), nil, nil},
	{[]byte("*0\r\n"), Array{}, nil},
	{[]byte("*1\r\n:10\r\n"), Array{int64(10)}, nil},
	{[]byte("*-1\r\n"), Array(nil), nil},
	{[]byte("*3\r\n+string\r\n-error\r\n:-2345\r\n"),
		Array{"string", "error", int64(-2345)}, nil},
	{[]byte("*5\r\n+string\r\n-error\r\n:-2345\r\n$4\r\nallo\r\n*2\r\n$0\r\n\r\n$-1\r\n"),
		Array{"string", "error", int64(-2345), "allo",
			Array{"", nil}}, nil},
}

var decodeRequestCases = []struct {
	raw []byte
	exp []string
	err error
}{
	{[]byte("*-1\r\n"), nil, ErrInvalidRequest},
	{[]byte(":4\r\n"), nil, ErrNotAnArray},
	{[]byte("*0\r\n"), nil, ErrInvalidRequest},
	{[]byte("*1\r\n:6\r\n"), nil, ErrInvalidRequest},
	{[]byte("*1\r\n$2\r\nab\r\n"), []string{"ab"}, nil},
	{[]byte("*3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$24\r\nceci n'est pas un string\r\n"),
		[]string{"SET", "mykey", "ceci n'est pas un string"}, nil},
}

func TestDecode(t *testing.T) {
	for _, c := range append(decodeValidCases, decodeErrCases...) {
		dec := NewDecoder(bytes.NewReader(c.enc))
		dec.maxLength = 60
		got, err := dec.Decode()
		if err != c.err {
			t.Errorf("%s: expected error %v, got %v", string(c.enc), c.err, err)
		}
		if got == nil && c.val == nil {
			continue
		}
		assertValue(t, string(c.enc), got, c.val)
	}
}

func TestDecodeRequest(t *testing.T) {
	for _, c := range decodeRequestCases {
		got, err := NewDecoder(bytes.NewReader(c.raw)).DecodeRequest()
		if err != c.err {
			t.Errorf("%s: expected error %v, got %v", string(c.raw), c.err, err)
		}
		if got == nil && c.exp == nil {
			continue
		}
		assertValue(t, string(c.raw), got, c.exp)
	}
}

func assertValue(t *testing.T, in string, got, exp interface{}) {
	tgot, texp := reflect.TypeOf(got), reflect.TypeOf(exp)
	if tgot != texp {
		t.Errorf("%s: expected type %s, got %s", in, texp, tgot)
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("%s: expected output %v, got %v", in, exp, got)
	}
}

var forbenchmark interface{}

func BenchmarkDecodeSimpleString(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeValidCases[3].enc)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}

func BenchmarkDecodeError(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeValidCases[7].enc)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}

func BenchmarkDecodeInteger(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeValidCases[10].enc)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}

func BenchmarkDecodeBulkString(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeValidCases[13].enc)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}

func BenchmarkDecodeArray(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeValidCases[19].enc)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}

func BenchmarkDecodeRequest(b *testing.B) {
	var val interface{}
	var err error

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(decodeRequestCases[5].raw)
		val, err = NewDecoder(r).Decode()
	}
	if err != nil {
		b.Fatal(err)
	}
	forbenchmark = val
}
