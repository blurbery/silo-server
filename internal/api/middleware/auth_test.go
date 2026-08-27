package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Silo-Server/silo-server/internal/auth"
	"github.com/Silo-Server/silo-server/internal/streamtoken"
)

type transportAuthTokenValidator struct {
	claims *auth.Claims
	err    error
	calls  int
}

func (v *transportAuthTokenValidator) ValidateToken(string) (*auth.Claims, error) {
	v.calls++
	return v.claims, v.err
}

type transportAuthSessionValidator struct {
	valid bool
}

func (v transportAuthSessionValidator) IsValid(context.Context, string) (bool, error) {
	return v.valid, nil
}

func TestRequireTransportAuthUsesSessionBoundCapabilityWhenAccessTokenExpired(t *testing.T) {
	const secret = "transport-auth-test-secret"
	validator := &transportAuthTokenValidator{err: errors.New("expired access token")}
	middleware := NewAuthMiddleware(validator, transportAuthSessionValidator{}, nil, nil)
	token := signTransportAuthToken(t, secret, "playback-1", 7, 42)

	router := chi.NewRouter()
	handler := func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		streamClaims := GetTransportStreamClaims(r.Context())
		if claims == nil || claims.UserID != 7 || claims.TokenType != auth.TokenTypeStream {
			t.Fatalf("auth claims = %#v, want stream user 7", claims)
		}
		if streamClaims == nil || streamClaims.SessionID != "playback-1" || streamClaims.MediaFileID != 42 {
			t.Fatalf("stream claims = %#v, want bound playback capability", streamClaims)
		}
		w.WriteHeader(http.StatusNoContent)
	}
	router.With(middleware.RequireTransportAuth(secret)).Get("/stream/{session_id}", handler)
	router.With(middleware.RequireTransportAuth(secret)).Head("/stream/{session_id}", handler)

	for _, carrier := range []struct {
		name  string
		apply func(*http.Request)
	}{
		{
			name: "query",
			apply: func(req *http.Request) {
				req.URL.RawQuery = streamtoken.QueryParameter + "=" + url.QueryEscape(token)
			},
		},
		{
			name: "header",
			apply: func(req *http.Request) {
				req.Header.Set(streamtoken.Header, token)
			},
		},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req := httptest.NewRequest(method, "/stream/playback-1", nil)
			carrier.apply(req)
			req.Header.Set("Authorization", "Bearer expired-access-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("%s %s status = %d, body = %s", carrier.name, method, rec.Code, rec.Body.String())
			}
		}
	}
	if validator.calls != 0 {
		t.Fatalf("access validator calls = %d, want stream capability hot path", validator.calls)
	}
}

func TestRequireTransportAuthRejectsCapabilityBoundToAnotherSession(t *testing.T) {
	const secret = "transport-auth-test-secret"
	validator := &transportAuthTokenValidator{err: errors.New("expired access token")}
	middleware := NewAuthMiddleware(validator, transportAuthSessionValidator{}, nil, nil)
	token := signTransportAuthToken(t, secret, "playback-other", 7, 42)

	router := chi.NewRouter()
	router.With(middleware.RequireTransportAuth(secret)).Get("/stream/{session_id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/stream/playback-1?"+streamtoken.QueryParameter+"="+url.QueryEscape(token), nil)
	req.Header.Set("Authorization", "Bearer expired-access-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if validator.calls != 1 {
		t.Fatalf("access validator calls = %d, want fallback auth attempt", validator.calls)
	}
}

func TestRequireTransportAuthRejectsCapabilityWithEmptySecretOrDownloadMethod(t *testing.T) {
	const configuredSecret = "transport-auth-test-secret"
	tests := []struct {
		name       string
		secret     string
		token      func(*testing.T) string
		wantStatus int
	}{
		{
			name:   "empty configured secret",
			secret: "",
			token: func(t *testing.T) string {
				t.Helper()
				return signTransportAuthToken(t, configuredSecret, "playback-1", 7, 42)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "download capability",
			secret: configuredSecret,
			token: func(t *testing.T) string {
				t.Helper()
				token, err := streamtoken.Sign(streamtoken.Claims{
					SessionID:   "playback-1",
					UserID:      7,
					MediaFileID: 42,
					PlayMethod:  streamtoken.PlayMethodDownload,
				}, configuredSecret, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				return token
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "expired playback capability",
			secret: configuredSecret,
			token: func(t *testing.T) string {
				t.Helper()
				token, err := streamtoken.Sign(streamtoken.Claims{
					SessionID:   "playback-1",
					UserID:      7,
					MediaFileID: 42,
					PlayMethod:  "direct",
				}, configuredSecret, -time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				return token
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validator := &transportAuthTokenValidator{err: errors.New("invalid access token")}
			middleware := NewAuthMiddleware(validator, transportAuthSessionValidator{}, nil, nil)
			router := chi.NewRouter()
			router.With(middleware.RequireTransportAuth(test.secret)).Get("/stream/{session_id}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/stream/playback-1?"+streamtoken.QueryParameter+"="+url.QueryEscape(test.token(t)), nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, test.wantStatus)
			}
		})
	}
}

func signTransportAuthToken(t *testing.T, secret, sessionID string, userID, mediaFileID int) string {
	t.Helper()
	token, err := streamtoken.Sign(streamtoken.Claims{
		SessionID:   sessionID,
		UserID:      userID,
		ProfileID:   "profile-1",
		MediaFileID: mediaFileID,
		PlayMethod:  "direct",
	}, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
