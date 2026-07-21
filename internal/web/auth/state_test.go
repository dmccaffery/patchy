// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"
)

func TestSealUnsealRoundTrip(t *testing.T) {
	key, err := sealKey("client-secret", "test")
	if err != nil {
		t.Fatalf("sealKey: %v", err)
	}
	in := loginState{Verifier: "v", CSRF: "c", Nonce: "n", OriginalPath: "/finding/x"}
	blob, err := seal(key, in)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	var out loginState
	if err := unseal(key, blob, &out); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if out != in {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestUnsealRejectsBadInput(t *testing.T) {
	key, _ := sealKey("client-secret", "test")
	otherKey, _ := sealKey("rotated-secret", "test")
	blob, err := seal(key, loginState{CSRF: "c"})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	tampered := blob[:len(blob)-2] + "zz"

	cases := []struct {
		name string
		key  []byte
		blob string
	}{
		{"tampered ciphertext", key, tampered},
		{"rotated key", otherKey, blob},
		{"not base64", key, "!!!"},
		{"too short", key, "aaaa"},
		{"empty", key, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out loginState
			if err := unseal(tc.key, tc.blob, &out); err == nil {
				t.Error("unseal accepted bad input")
			}
		})
	}
}

func TestSealKeyDomainSeparation(t *testing.T) {
	a, _ := sealKey("secret", "session")
	b, _ := sealKey("secret", "other")
	if string(a) == string(b) {
		t.Error("different info strings derived the same key")
	}
}

func TestRandomToken(t *testing.T) {
	a, err1 := randomToken()
	b, err2 := randomToken()
	if err1 != nil || err2 != nil {
		t.Fatalf("randomToken: %v/%v", err1, err2)
	}
	if a == b || len(a) < 40 || strings.ContainsAny(a, "+/=") {
		t.Errorf("tokens %q/%q not distinct URL-safe strings", a, b)
	}
}
