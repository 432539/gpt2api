package jwtpayload

import "testing"

func TestExpUnixFromJWT(t *testing.T) {
	// payload: {"exp":1735689600}  (base64url)
	const tok = "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjE3MzU2ODk2MDB9.signature"
	exp, ok := ExpUnixFromJWT(tok)
	if !ok || exp != 1735689600 {
		t.Fatalf("got %d ok=%v", exp, ok)
	}
	if _, ok := ExpUnixFromJWT("nope"); ok {
		t.Fatal("expected false")
	}
}
