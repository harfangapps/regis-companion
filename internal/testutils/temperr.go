package testutils

import "fmt"

var (
	// ErrTemporaryTrue is an error that implements a Temporary method
	// that always returns true.
	ErrTemporaryTrue tempErr = true

	// ErrTemporaryFalse is an error that implements a Temporary method
	// that always returns false.
	ErrTemporaryFalse tempErr = false
)

type tempErr bool

func (e tempErr) Temporary() bool {
	return bool(e)
}

func (e tempErr) Error() string {
	return fmt.Sprintf("temporary error %t", bool(e))
}
