package testutils

// TempErr is an error that implements the Temporary method.
var TempErr tempErr = "temporary error"

type tempErr string

func (e tempErr) Temporary() bool {
	return true
}

func (e tempErr) Error() string {
	return string(e)
}
