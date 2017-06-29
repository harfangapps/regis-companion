// Package resp implements an efficient decoder for the Redis Serialization Protocol (RESP).
//
// See http://redis.io/topics/protocol for the reference.
package resp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidPrefix is returned if the data contains an unrecognized prefix.
	ErrInvalidPrefix = errors.New("resp: invalid prefix")

	// ErrMissingCRLF is returned if a \r\n is missing in the data slice.
	ErrMissingCRLF = errors.New("resp: missing CRLF")

	// ErrInvalidInteger is returned if an invalid character is found while parsing an integer.
	ErrInvalidInteger = errors.New("resp: invalid integer character")

	// ErrInvalidBulkString is returned if the bulk string data cannot be decoded.
	ErrInvalidBulkString = errors.New("resp: invalid bulk string")

	// ErrInvalidArray is returned if the array data cannot be decoded.
	ErrInvalidArray = errors.New("resp: invalid array")

	// ErrNotAnArray is returned if the DecodeRequest function is called and
	// the decoded value is not an array.
	ErrNotAnArray = errors.New("resp: expected an array type")

	// ErrInvalidRequest is returned if the DecodeRequest function is called and
	// the decoded value is not an array containing only bulk strings, and at least 1 element.
	ErrInvalidRequest = errors.New("resp: invalid request, must be an array of bulk strings with at least one element")
)

const (
	defaultMaxLine   = 4096      // 4KB
	defaultMaxLength = 512 << 20 // 512MB
)

// Decoder decodes values received by an io.Reader.
type Decoder struct {
	r         *bufio.Reader
	buf       bytes.Buffer
	limit     io.LimitedReader
	maxLine   int
	maxLength int
}

// NewDecoder returns a new Decoder that reads values from r.
func NewDecoder(r io.Reader) *Decoder {
	dec := &Decoder{
		r:         bufferedReader(r),
		maxLine:   defaultMaxLine,
		maxLength: defaultMaxLength,
	}
	return dec
}

func bufferedReader(r io.Reader) *bufio.Reader {
	if br, ok := r.(*bufio.Reader); ok {
		return br
	}
	return bufio.NewReader(r)
}

// Array represents an array of values, as defined by the RESP.
type Array []interface{}

// String is the Stringer implementation for the Array.
func (a Array) String() string {
	var buf bytes.Buffer
	for i, v := range a {
		buf.WriteString(fmt.Sprintf("[%2d] %[2]v (%[2]T)\n", i, v))
	}
	return buf.String()
}

// DecodeRequest decodes the provided byte slice and returns the array
// representing the request. If the encoded value is not an array, it
// returns ErrNotAnArray, and if it is not a valid request, it returns ErrInvalidRequest.
func (d *Decoder) DecodeRequest() ([]string, error) {
	// Decode the value, must be an array
	val, err := d.decodeValue(true)
	if err != nil {
		return nil, err
	}
	ar, ok := val.(Array)
	if !ok {
		return nil, ErrNotAnArray
	}

	// Must have at least one element
	if len(ar) < 1 {
		return nil, ErrInvalidRequest
	}

	// Must have only strings
	strs := make([]string, len(ar))
	for i, v := range ar {
		v, ok := v.(string)
		if !ok {
			return nil, ErrInvalidRequest
		}
		strs[i] = v
	}
	return strs, nil
}

// Decode decodes the provided byte slice and returns the parsed value.
func (d *Decoder) Decode() (interface{}, error) {
	return d.decodeValue(false)
}

// decodeValue parses the byte slice and decodes the value based on its
// prefix, as defined by the RESP protocol.
func (d *Decoder) decodeValue(requiresArray bool) (interface{}, error) {
	ch, err := d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if requiresArray && ch != '*' {
		return nil, ErrNotAnArray
	}

	var val interface{}
	switch ch {
	case '+':
		// Simple string
		val, err = d.decodeSimpleString()
	case '-':
		// Error
		val, err = d.decodeError()
	case ':':
		// Integer
		val, err = d.decodeInteger()
	case '$':
		// Bulk string
		val, err = d.decodeBulkString()
	case '*':
		// Array
		val, err = d.decodeArray()
	default:
		err = ErrInvalidPrefix
	}

	return val, err
}

// decodeArray decodes the byte slice as an array. It assumes the
// '*' prefix is already consumed.
func (d *Decoder) decodeArray() (Array, error) {
	// First comes the number of elements in the array
	cnt, err := d.decodeInteger()
	if err != nil {
		return nil, err
	}
	switch {
	case cnt == -1:
		// Nil array
		return nil, nil

	case cnt == 0:
		// Empty, but allocated, array
		return Array{}, nil

	case cnt < 0:
		// Invalid length
		return nil, ErrInvalidArray

		// TODO: cnt > 512MB

	default:
		// Allocate the array
		ar := make(Array, cnt)

		// Decode each value
		for i := 0; i < int(cnt); i++ {
			val, err := d.decodeValue(false)
			if err != nil {
				return nil, err
			}
			ar[i] = val
		}
		return ar, nil
	}
}

// decodeBulkString decodes the byte slice as a binary-safe string. The
// '$' prefix is assumed to be already consumed.
func (d *Decoder) decodeBulkString() (interface{}, error) {
	// First comes the length of the bulk string, an integer
	cnt, err := d.decodeInteger()
	if err != nil {
		return nil, err
	}
	switch {
	case cnt == -1:
		// Special case to represent a nil bulk string
		return nil, nil

	case cnt < -1:
		return nil, ErrInvalidBulkString

		// TODO: cnt > 512MB

	default:
		// Then the string is cnt long, and bytes read is cnt+n+2 (for ending CRLF)
		need := cnt + 2
		got := 0
		// TODO: reuse scratch space instead
		buf := make([]byte, need)
		// TODO: use io.ReadFull
		for {
			nb, err := d.r.Read(buf[got:])
			if err != nil {
				return nil, err
			}
			got += nb
			if int64(got) == need {
				break
			}
		}
		return string(buf[:got-2]), err
	}
}

// decodeInteger decodes the byte slice as a singed 64bit integer. The
// ':' prefix is assumed to be already consumed.
func (d *Decoder) decodeInteger() (val int64, err error) {
	var cr bool
	var sign int64 = 1
	var n int

loop:
	for {
		// TODO: limit to n characters (int64 + sign)

		ch, err := d.r.ReadByte()
		if err != nil {
			return 0, err
		}
		n++

		switch ch {
		case '\r':
			cr = true
			break loop

		case '\n':
			break loop

		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			val = val*10 + int64(ch-'0')

		case '-':
			if n == 1 {
				sign = -1
				continue
			}
			fallthrough
		default:
			return 0, ErrInvalidInteger
		}
	}

	if !cr {
		return 0, ErrMissingCRLF
	}
	// Presume next byte was \n
	// TODO: do not presume
	_, err = d.r.ReadByte()
	if err != nil {
		return 0, err
	}
	return sign * val, nil
}

// decodeSimpleString decodes the byte slice as a SimpleString. The
// '+' prefix is assumed to be already consumed.
func (d *Decoder) decodeSimpleString() (interface{}, error) {
	// TODO: use limit reader
	v, err := d.r.ReadBytes('\r')
	if err != nil {
		return nil, err
	}
	// Presume next byte was \n
	// TODO: do not presume
	_, err = d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	return string(v[:len(v)-1]), nil
}

// decodeError decodes the byte slice as an Error. The '-' prefix
// is assumed to be already consumed.
func (d *Decoder) decodeError() (interface{}, error) {
	return d.decodeSimpleString()
}
