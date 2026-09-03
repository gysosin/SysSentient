package auth

import "testing"

func TestNewTokenIsUniqueAndURLSafe(t *testing.T) {
	t.Parallel()
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("two tokens were identical")
	}
	if len(a) != 43 {
		t.Fatalf("token length = %d, want 43 (32 bytes base64url, unpadded)", len(a))
	}
	for _, c := range a {
		if c == '+' || c == '/' || c == '=' {
			t.Fatalf("token contains non-URL-safe char %q", c)
		}
	}
}

func TestHashTokenIsDeterministicHex(t *testing.T) {
	t.Parallel()
	h1 := HashToken("abc")
	h2 := HashToken("abc")
	if h1 != h2 {
		t.Fatal("HashToken not deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(h1))
	}
	if h1 == "abc" {
		t.Fatal("HashToken returned its input")
	}
}

func TestNewIDIs32Hex(t *testing.T) {
	t.Parallel()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("id length = %d, want 32", len(id))
	}
}
