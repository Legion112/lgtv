package webos

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointsPrefersPlaintextPort(t *testing.T) {
	eps := endpoints("192.168.8.187", false)
	if len(eps) != 1 {
		t.Fatalf("endpoints without -insecure returned %d candidates, want 1", len(eps))
	}
	if want := "ws://192.168.8.187:3000/"; eps[0].url != want {
		t.Errorf("url = %q, want %q", eps[0].url, want)
	}
}

func TestEndpointsAddsTLSWhenInsecure(t *testing.T) {
	eps := endpoints("192.168.8.187", true)
	if len(eps) != 2 {
		t.Fatalf("endpoints with -insecure returned %d candidates, want 2", len(eps))
	}
	if !strings.HasPrefix(eps[0].url, "ws://") {
		t.Errorf("first candidate = %q, want the plaintext port tried first", eps[0].url)
	}
	if want := "wss://192.168.8.187:3001/"; eps[1].url != want {
		t.Errorf("second candidate = %q, want %q", eps[1].url, want)
	}
	if eps[1].client == nil {
		t.Error("TLS candidate has no HTTP client, so certificate verification would not be skipped")
	}
}

func TestEndpointsHonoursExplicitPort(t *testing.T) {
	eps := endpoints("192.168.8.187:9000", true)
	if len(eps) != 1 {
		t.Fatalf("an explicit port produced %d candidates, want exactly 1", len(eps))
	}
	if want := "ws://192.168.8.187:9000/"; eps[0].url != want {
		t.Errorf("url = %q, want %q", eps[0].url, want)
	}
}

// Port 3001 is TLS-only, so naming it explicitly must not fall back to ws://.
func TestEndpointsUsesTLSForExplicitSecurePort(t *testing.T) {
	eps := endpoints("tv.local:3001", false)
	if len(eps) != 1 {
		t.Fatalf("got %d candidates, want 1", len(eps))
	}
	if want := "wss://tv.local:3001/"; eps[0].url != want {
		t.Errorf("url = %q, want %q", eps[0].url, want)
	}
}

func TestEndpointsBracketsIPv6(t *testing.T) {
	eps := endpoints("fe80::1", false)
	if want := "ws://[fe80::1]:3000/"; eps[0].url != want {
		t.Errorf("url = %q, want %q", eps[0].url, want)
	}
}

func TestKeyStoreKeepsOneFilePerHost(t *testing.T) {
	store := &KeyStore{dir: t.TempDir()}

	if got := store.Key("192.168.8.187"); got != "" {
		t.Errorf("Key() on an empty store = %q, want empty", got)
	}
	if err := store.SaveKey("192.168.8.187", "key-a"); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	if err := store.SaveKey("192.168.8.50", "key-b"); err != nil {
		t.Fatalf("SaveKey: %v", err)
	}
	if got := store.Key("192.168.8.187"); got != "key-a" {
		t.Errorf("Key(.187) = %q, want %q", got, "key-a")
	}
	if got := store.Key("192.168.8.50"); got != "key-b" {
		t.Errorf("Key(.50) = %q, want %q — keys must not overwrite each other", got, "key-b")
	}
}

func TestKeyStoreRoundTripsDefaultHost(t *testing.T) {
	store := &KeyStore{dir: t.TempDir()}
	if err := store.SaveHost("192.168.8.187"); err != nil {
		t.Fatalf("SaveHost: %v", err)
	}
	if got := store.Host(); got != "192.168.8.187" {
		t.Errorf("Host() = %q, want %q", got, "192.168.8.187")
	}
}

// A host is attacker-influenced only in the sense that it may be a hostname
// with odd characters; the key file must still stay inside the store.
func TestKeyStorePathStaysInsideDir(t *testing.T) {
	dir := t.TempDir()
	store := &KeyStore{dir: dir}
	for _, host := range []string{"../escape", "a/b", `c\d`, "..", "tv.local:3001"} {
		path := store.keyPath(host)
		if filepath.Dir(path) != dir {
			t.Errorf("keyPath(%q) = %q, which escapes %q", host, path, dir)
		}
	}
}

func TestSSDPHeaderIsCaseInsensitive(t *testing.T) {
	resp := []byte("HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"Location: http://192.168.8.187:1754/description.xml\r\n" +
		"ST: urn:lge-com:service:webos-second-screen:1\r\n\r\n")

	if got := ssdpHeader(resp, "LOCATION"); got != "http://192.168.8.187:1754/description.xml" {
		t.Errorf("ssdpHeader(LOCATION) = %q", got)
	}
	if got := ssdpHeader(resp, "missing"); got != "" {
		t.Errorf("ssdpHeader(missing) = %q, want empty", got)
	}
}
