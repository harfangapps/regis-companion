package addr

import (
	"net"
	"testing"
)

func TestHostPortAddrEquality(t *testing.T) {
	cases := []struct {
		a, b net.Addr
		want bool
	}{
		{a: HostPortAddr{}, b: HostPortAddr{}, want: true},
		{a: HostPortAddr{Host: "a"}, b: HostPortAddr{}, want: false},
		{a: HostPortAddr{Host: "a"}, b: HostPortAddr{Host: "b"}, want: false},
		{a: HostPortAddr{Host: "a"}, b: HostPortAddr{Host: "a"}, want: true},
		{a: HostPortAddr{Port: 1}, b: HostPortAddr{Host: "a"}, want: false},
		{a: HostPortAddr{Port: 1}, b: HostPortAddr{}, want: false},
		{a: HostPortAddr{Port: 1}, b: HostPortAddr{Port: 1}, want: true},
		{a: HostPortAddr{Port: 1}, b: HostPortAddr{Port: 2}, want: false},
		{a: HostPortAddr{Host: "a", Port: 1}, b: HostPortAddr{Host: "a", Port: 2}, want: false},
		{a: HostPortAddr{Host: "a", Port: 2}, b: HostPortAddr{Host: "a", Port: 2}, want: true},
		{a: HostPortAddr{Host: "b", Port: 2}, b: HostPortAddr{Host: "a", Port: 2}, want: false},
		{a: HostPortAddr{Host: "a", Port: 1}, b: &net.TCPAddr{}, want: false},
		{a: HostPortAddr{Host: "10.0.0.1", Port: 1}, b: &net.TCPAddr{IP: []byte{10, 0, 0, 1}, Port: 1}, want: false},
	}

	for _, c := range cases {
		got := c.a == c.b
		if got != c.want {
			t.Errorf("%s %s == %s %s -> want %v, got %v", c.a.Network(), c.a.String(), c.b.Network(), c.b.String(), c.want, got)
		}
	}
}
