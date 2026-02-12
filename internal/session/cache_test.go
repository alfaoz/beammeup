package session

import "testing"

func TestPasswordCacheLifecycle(t *testing.T) {
	cache := NewPasswordCache()

	if _, ok := cache.Get("rps"); ok {
		t.Fatal("expected cache miss")
	}

	cache.Set("rps", "secret")
	if v, ok := cache.Get("rps"); !ok || v != "secret" {
		t.Fatalf("unexpected cache value ok=%v v=%q", ok, v)
	}

	cache.Forget("rps")
	if _, ok := cache.Get("rps"); ok {
		t.Fatal("expected miss after forget")
	}

	cache.Set("a", "1")
	cache.Set("b", "2")
	cache.Clear()
	if _, ok := cache.Get("a"); ok {
		t.Fatal("expected empty cache after clear")
	}
}
