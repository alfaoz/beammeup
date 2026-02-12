package cli

import "testing"

func TestNormalizeProtocol(t *testing.T) {
	cases := map[string]string{
		"http":   "http",
		"socks5": "socks5",
		"socks":  "socks5",
		"":       "",
	}
	for in, want := range cases {
		got, ok := NormalizeProtocol(in)
		if !ok {
			t.Fatalf("expected protocol %q to be valid", in)
		}
		if got != want {
			t.Fatalf("NormalizeProtocol(%q)=%q want %q", in, got, want)
		}
	}
	if _, ok := NormalizeProtocol("invalid"); ok {
		t.Fatal("expected invalid protocol")
	}
}

func TestNormalizeAction(t *testing.T) {
	cases := map[string]string{
		"show":      "show",
		"configure": "configure",
		"rotate":    "rotate",
		"destroy":   "destroy",
		"install":   "configure",
		"uninstall": "destroy",
		"":          "",
	}
	for in, want := range cases {
		got, ok := NormalizeAction(in)
		if !ok {
			t.Fatalf("expected action %q to be valid", in)
		}
		if got != want {
			t.Fatalf("NormalizeAction(%q)=%q want %q", in, got, want)
		}
	}
	if _, ok := NormalizeAction("oops"); ok {
		t.Fatal("expected invalid action")
	}
}

func TestNormalizeHTTPMode(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"auto":    "",
		"sidecar": "sidecar",
	}
	for in, want := range cases {
		got, ok := NormalizeHTTPMode(in)
		if !ok {
			t.Fatalf("expected mode %q to be valid", in)
		}
		if got != want {
			t.Fatalf("NormalizeHTTPMode(%q)=%q want %q", in, got, want)
		}
	}
	if _, ok := NormalizeHTTPMode("managed"); ok {
		t.Fatal("expected invalid mode")
	}
}
