package addr

import "testing"

func TestListen(t *testing.T) {
	l, port, err := Listen(HostPortAddr{Host: "localhost", Port: 0})
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	defer l.Close()
	if port <= 1024 || port > 65535 {
		t.Fatalf("want valid port, got %v", port)
	}
	t.Logf("got port %d", port)
}
