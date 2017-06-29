package resp

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

var (
	// Common encoding values optimized to avoid allocations.
	pong = []byte("+PONG\r\n")
	ok   = []byte("+OK\r\n")
	t    = []byte(":1\r\n")
	f    = []byte(":0\r\n")
	one  = t
	zero = f
)

// ErrInvalidValue is returned if the value to encode is invalid.
var ErrInvalidValue = errors.New("resp: invalid value")

// Error represents an error string as defined by the RESP. It cannot
// contain \r or \n characters. It must be used as a type conversion
// so that Encode serializes the string as an Error.
type Error string

// Pong is a sentinel type used to indicate that the PONG simple string
// value should be encoded.
type Pong struct{}

// OK is a sentinel type used to indicate that the OK simple string
// value should be encoded.
type OK struct{}

// SimpleString represents a simple string as defined by the RESP. It
// cannot contain \r or \n characters. It must be used as a type conversion
// so that Encode serializes the string as a SimpleString.
type SimpleString string

// BulkString represents a binary-safe string as defined by the RESP.
// It can be used as a type conversion so that Encode serializes the string
// as a BulkString, but this is the default encoding for a normal Go string.
type BulkString string

// Encoder encodes values to the Redis serialization protocol.
type Encoder struct {
	w         *bufio.Writer
	maxLength int
}

// NewEncoder returns a new Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w:         bufferedWriter(w),
		maxLength: defaultMaxLength,
	}
}

func bufferedWriter(w io.Writer) *bufio.Writer {
	if bw, ok := w.(*bufio.Writer); ok {
		return bw
	}
	return bufio.NewWriter(w)
}

// Encode encodes the value v.
func (e *Encoder) Encode(v interface{}) error {
	if err := e.encodeValue(v); err != nil {
		return err
	}
	return e.w.Flush()
}

// TODO: use reusable scratch space and strconv.AppendXxx, write to a buffered writer
// and flush on exit?

func (e *Encoder) encodeValue(v interface{}) error {
	switch v := v.(type) {
	case OK:
		_, err := e.w.Write(ok)
		return err
	case Pong:
		_, err := e.w.Write(pong)
		return err
	case bool:
		if v {
			_, err := e.w.Write(t)
			return err
		}
		_, err := e.w.Write(f)
		return err
	case SimpleString:
		return e.encodeSimpleString(v)
	case Error:
		return e.encodeError(v)
	case int64:
		switch v {
		case 0:
			_, err := e.w.Write(zero)
			return err
		case 1:
			_, err := e.w.Write(one)
			return err
		default:
			return e.encodeInteger(v)
		}
	case string:
		return e.encodeBulkString(BulkString(v))
	case BulkString:
		return e.encodeBulkString(v)
	case []string:
		return e.encodeStringArray(v)
	case []interface{}:
		return e.encodeArray(Array(v))
	case Array:
		return e.encodeArray(v)
	case nil:
		return e.encodeNil()
	default:
		return ErrInvalidValue
	}
}

// encodeStringArray is a specialized array encoding func to avoid having to
// allocate an empty slice interface and copy values to it to use encodeArray.
func (e *Encoder) encodeStringArray(v []string) error {
	// Special case for a nil array
	if v == nil {
		return e.encodePrefixed('*', "-1")
	}

	// First encode the number of elements
	n := len(v)
	if err := e.encodePrefixed('*', strconv.Itoa(n)); err != nil {
		return err
	}

	// Then encode each value
	for _, el := range v {
		if err := e.encodeBulkString(BulkString(el)); err != nil {
			return err
		}
	}
	return nil
}

// encodeArray encodes an array value to w.
func (e *Encoder) encodeArray(v Array) error {
	// Special case for a nil array
	if v == nil {
		return e.encodePrefixed('*', "-1")
	}

	// First encode the number of elements
	n := len(v)
	if err := e.encodePrefixed('*', strconv.Itoa(n)); err != nil {
		return err
	}

	// Then encode each value
	for _, el := range v {
		if err := e.encodeValue(el); err != nil {
			return err
		}
	}
	return nil
}

// encodeBulkString encodes a bulk string to w.
func (e *Encoder) encodeBulkString(v BulkString) error {
	n := len(v)
	data := strconv.Itoa(n) + "\r\n" + string(v)
	return e.encodePrefixed('$', data)
}

// encodeInteger encodes an integer value to w.
func (e *Encoder) encodeInteger(v int64) error {
	return e.encodePrefixed(':', strconv.FormatInt(v, 10))
}

// encodeSimpleString encodes a simple string value to w.
func (e *Encoder) encodeSimpleString(v SimpleString) error {
	return e.encodePrefixed('+', string(v))
}

// encodeError encodes an error value to w.
func (e *Encoder) encodeError(v Error) error {
	return e.encodePrefixed('-', string(v))
}

// encodeNil encodes a nil value as a nil bulk string.
func (e *Encoder) encodeNil() error {
	return e.encodePrefixed('$', "-1")
}

// encodePrefixed encodes the data v to w, with the specified prefix.
func (e *Encoder) encodePrefixed(prefix byte, v string) error {
	// TODO: reuse scratch space
	buf := make([]byte, len(v)+3)
	buf[0] = prefix
	copy(buf[1:], v)
	copy(buf[len(buf)-2:], "\r\n")
	_, err := e.w.Write(buf)
	return err
}
