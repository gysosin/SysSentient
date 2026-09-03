package auth

import "testing"

func TestSetupTokenConsumesExactlyOnce(t *testing.T) {
	t.Parallel()
	tok, err := NewSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok.Consume("wrong") {
		t.Fatal("wrong token consumed")
	}
	if !tok.Consume(tok.String()) {
		t.Fatal("correct token rejected")
	}
	if tok.Consume(tok.String()) {
		t.Fatal("token consumed twice")
	}
}
