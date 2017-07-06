package resp

import (
	"bytes"
	"testing"
	"time"
)

var encodeValidCases = []struct {
	enc []byte
	val interface{}
	err error
}{
	{[]byte{'+', 'O', 'K', '\r', '\n'}, OK{}, nil},
	{[]byte{'+', 'P', 'O', 'N', 'G', '\r', '\n'}, Pong{}, nil},
	{[]byte{'+', '\r', '\n'}, SimpleString(""), nil},
	{[]byte{'+', 'a', '\r', '\n'}, SimpleString("a"), nil},
	{[]byte{'+', 'O', 'K', '\r', '\n'}, SimpleString("OK"), nil},
	{[]byte("+ceci n'est pas un string\r\n"), SimpleString("ceci n'est pas un string"), nil},
	{[]byte{'-', '\r', '\n'}, Error(""), nil},
	{[]byte{'-', 'a', '\r', '\n'}, Error("a"), nil},
	{[]byte{'-', 'K', 'O', '\r', '\n'}, Error("KO"), nil},
	{[]byte("-ceci n'est pas un string\r\n"), Error("ceci n'est pas un string"), nil},
	{[]byte(":0\r\n"), false, nil},
	{[]byte(":1\r\n"), true, nil},
	{[]byte(":0\r\n"), int64(0), nil},
	{[]byte(":1\r\n"), int64(1), nil},
	{[]byte(":123\r\n"), int64(123), nil},
	{[]byte(":-123\r\n"), int64(-123), nil},
	{[]byte("$0\r\n\r\n"), "", nil},
	{[]byte("$24\r\nceci n'est pas un string\r\n"), "ceci n'est pas un string", nil},
	{[]byte("$24\r\nceci n'est pas un string\r\n"), BulkString("ceci n'est pas un string"), nil},
	{[]byte("$51\r\nceci n'est pas un string\r\navec\rdes\nsauts\r\nde\x00ligne.\r\n"), "ceci n'est pas un string\r\navec\rdes\nsauts\r\nde\x00ligne.", nil},
	{[]byte("$-1\r\n"), nil, nil},
	{[]byte("*0\r\n"), Array{}, nil},
	{[]byte("*1\r\n:10\r\n"), Array{int64(10)}, nil},
	{[]byte("*1\r\n$2\r\nab\r\n"), []string{"ab"}, nil},
	{[]byte("*-1\r\n"), Array(nil), nil},
	{[]byte("*3\r\n+string\r\n-error\r\n:-2345\r\n"),
		Array{SimpleString("string"), Error("error"), int64(-2345)}, nil},
	{[]byte("*5\r\n+string\r\n-error\r\n:-2345\r\n$4\r\nallo\r\n*2\r\n$0\r\n\r\n$-1\r\n"),
		Array{SimpleString("string"), Error("error"), int64(-2345), "allo",
			Array{"", nil}}, nil},
	{nil, time.Second, ErrInvalidValue},
}

func TestEncode(t *testing.T) {
	var buf bytes.Buffer
	var err error

	for _, c := range encodeValidCases {
		buf.Reset()
		enc := NewEncoder(&buf)
		err = enc.Encode(c.val)
		if err != c.err {
			t.Errorf("%s: expected error %v, got %v", c.val, c.err, err)
		}
		if err == nil {
			if !bytes.Equal(buf.Bytes(), c.enc) {
				t.Errorf("%v: expected %x (%q), got %x (%q)", c.val, c.enc, string(c.enc), buf.Bytes(), buf.String())
			}
		}
	}
}

func BenchmarkEncodeSimpleString(b *testing.B) {
	var err error
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < b.N; i++ {
		err = enc.Encode(encodeValidCases[3].val)
	}
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkEncodeError(b *testing.B) {
	var err error
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < b.N; i++ {
		err = enc.Encode(encodeValidCases[7].val)
	}
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkEncodeInteger(b *testing.B) {
	var err error
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < b.N; i++ {
		err = enc.Encode(encodeValidCases[10].val)
	}
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkEncodeBulkString(b *testing.B) {
	var err error
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < b.N; i++ {
		err = enc.Encode(encodeValidCases[13].val)
	}
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkEncodeArray(b *testing.B) {
	var err error
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < b.N; i++ {
		err = enc.Encode(encodeValidCases[19].val)
	}
	if err != nil {
		b.Fatal(err)
	}
}
