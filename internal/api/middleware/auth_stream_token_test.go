package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Silo-Server/silo-server/internal/auth"
	"github.com/Silo-Server/silo-server/internal/streamtoken"
)

type rejectedAccessTokenValidator struct{}

func (rejectedAccessTokenValidator) ValidateToken(string) (*auth.Claims, error) {
	return nil, errors.New("expired access token")
}

type acceptedAccessTokenValidator struct{}

func (acceptedAccessTokenValidator) ValidateToken(string) (*auth.Claims, error) {
	return &auth.Claims{
		UserID:    23,
		ProfileID: "profile-2",
		SessionID: "account-session-1",
		TokenType: auth.TokenTypeAccess,
	}, nil
}

type acceptedSessionValidator struct{}

func (acceptedSessionValidator) IsValid(context.Context, string) (bool, error) {
	return true, nil
}

func TestRequireAuthOrStreamTokenAcceptsSessionBoundTokenAfterAccountExpiry(t *testing.T) {
	const secret = "stream-token-test-secret"
	signed, err := streamtoken.Sign(streamtoken.Claims{
		SessionID: "playback-session-1",
		UserID:    17,
		ProfileID: "profile-1",
	}, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	authMiddleware := NewAuthMiddleware(rejectedAccessTokenValidator{}, acceptedSessionValidator{}, nil, nil)
	router := chi.NewRouter()
	router.With(authMiddleware.RequireAuthOrStreamToken(secret)).Get("/stream/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		if got := GetUserID(r.Context()); got != 17 {
			t.Fatalf("user id = %d, want 17", got)
		}
		if got := GetProfileID(r.Context()); got != "profile-1" {
			t.Fatalf("profile id = %q, want profile-1", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/stream/playback-session-1?st="+signed, nil)
	req.Header.Set("Authorization", "Bearer expired-account-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestRequireAuthOrStreamTokenRejectsTokenForAnotherSession(t *testing.T) {
	const secret = "stream-token-test-secret"
	signed, err := streamtoken.Sign(streamtoken.Claims{
		SessionID: "playback-session-1",
		UserID:    17,
	}, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	authMiddleware := NewAuthMiddleware(rejectedAccessTokenValidator{}, acceptedSessionValidator{}, nil, nil)
	router := chi.NewRouter()
	router.With(authMiddleware.RequireAuthOrStreamToken(secret)).Get("/stream/{session_id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/stream/playback-session-2?st="+signed, nil)
	req.Header.Set("Authorization", "Bearer expired-account-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestRequireAuthOrStreamTokenFallsBackToAccountAuth(t *testing.T) {
	authMiddleware := NewAuthMiddleware(acceptedAccessTokenValidator{}, acceptedSessionValidator{}, nil, nil)
	router := chi.NewRouter()
	router.With(authMiddleware.RequireAuthOrStreamToken("stream-token-test-secret")).Get("/stream/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		if got := GetUserID(r.Context()); got != 23 {
			t.Fatalf("user id = %d, want 23", got)
		}
		if claims := GetClaims(r.Context()); claims == nil || claims.ProfileID != "profile-2" {
			t.Fatalf("claims = %#v, want profile profile-2", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/stream/playback-session-2", nil)
	req.Header.Set("Authorization", "Bearer live-account-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
