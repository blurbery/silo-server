package streamtoken

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestStreamTokenRequiresSecretAndHS256(t *testing.T) {
	claims := Claims{SessionID: "playback-1", UserID: 7, MediaFileID: 42, PlayMethod: "direct"}
	if _, err := Sign(claims, "", time.Hour); err == nil {
		t.Fatal("Sign accepted an empty secret")
	}
	if _, err := Verify("not-a-token", ""); err == nil {
		t.Fatal("Verify accepted an empty secret")
	}

	hmac384 := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	token, err := hmac384.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(token, "secret"); err == nil {
		t.Fatal("Verify accepted a non-HS256 HMAC token")
	}

	hs256, err := Sign(claims, "secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(hs256, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if verified.SessionID != claims.SessionID || verified.UserID != claims.UserID || verified.MediaFileID != claims.MediaFileID {
		t.Fatalf("verified claims = %#v, want %#v", verified, claims)
	}
}
