package push

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	public := []string{"8.8.8.8", "1.1.1.1", "142.250.80.46", "2606:4700:4700::1111", "::ffff:8.8.8.8"}
	for _, s := range public {
		if !IsPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should be public", s)
		}
	}
	private := []string{
		"127.0.0.1", "127.1.2.3", "::1", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1",
		"169.254.169.254", "fe80::1", "fc00::1", "fd12:3456::1", "0.0.0.0", "::", "100.64.0.1",
		"224.0.0.1", "ff02::1", "240.0.0.1", "255.255.255.255", "192.0.2.1", "198.18.0.1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254", "64:ff9b::7f00:1",
	}
	for _, s := range private {
		if IsPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should NOT be public", s)
		}
	}
	if IsPublicIP(nil) {
		t.Error("nil is not public")
	}
}

func fakeResolver(m map[string][]string) Resolver {
	return func(_ context.Context, host string) ([]net.IP, error) {
		v, ok := m[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var out []net.IP
		for _, s := range v {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func TestValidateEndpoint(t *testing.T) {
	res := fakeResolver(map[string][]string{
		"fcm.googleapis.com": {"142.250.80.46", "2a00:1450::5f"},
		"internal.example":   {"10.0.0.5"},
		"mixed.example":      {"8.8.8.8", "127.0.0.1"},
	})
	good := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"HTTPS://fcm.googleapis.com/x",
		"https://8.8.8.8/x",
		"https://[2606:4700:4700::1111]/x",
	}
	for _, u := range good {
		if err := ValidateEndpoint(context.Background(), u, res); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	bad := []string{
		"", "not a url", "/relative", "fcm.googleapis.com/x",
		"http://fcm.googleapis.com/x", // http
		"ftp://fcm.googleapis.com/x",
		"javascript:alert(1)",
		"https://",
		"https://127.0.0.1/x",
		"https://[::1]/x",
		"https://10.0.0.1/x",
		"https://169.254.169.254/latest/meta-data",
		"https://[::ffff:127.0.0.1]/x",
		"https://localhost/x",
		"https://foo.localhost/x",
		"https://printer.local/x",
		"https://internal.example/x", // resolves private
		"https://mixed.example/x",    // any private answer rejects
		"https://nxdomain.example/x", // does not resolve
		"https://user:pw@fcm.googleapis.com/x",
		" https://fcm.googleapis.com/x",
	}
	for _, u := range bad {
		err := ValidateEndpoint(context.Background(), u, res)
		if !errors.Is(err, ErrInvalidEndpoint) {
			t.Errorf("%q: err = %v, want ErrInvalidEndpoint", u, err)
		}
	}
}

func TestSafeClientRefusesLoopback(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	_, err := NewSafeClient().Get(srv.URL)
	if err == nil || hit {
		t.Fatalf("safe client reached a loopback server (hit=%v err=%v)", hit, err)
	}
}
