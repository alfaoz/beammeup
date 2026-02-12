package remote

import "testing"

func TestParseBM(t *testing.T) {
	out := "foo\nBM_A=hello\nBM_B=true\nBM_C=42\n"
	kv := ParseBM(out)

	if got := kv.Get("BM_A"); got != "hello" {
		t.Fatalf("BM_A = %q", got)
	}
	if !kv.Bool("BM_B") {
		t.Fatalf("BM_B expected true")
	}
	if got := kv.Int("BM_C"); got != 42 {
		t.Fatalf("BM_C = %d", got)
	}
}
