package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Silo-Server/silo-server/internal/historyimport"
)

func TestHistoryImportUpstreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "unauthorized",
			status:     http.StatusUnauthorized,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
			wantMsg:    "Couldn't connect to that server. Check the URL, username, and password and try again.",
		},
		{
			name:       "bad request",
			status:     http.StatusBadRequest,
			wantStatus: http.StatusBadRequest,
			wantCode:   "bad_request",
			wantMsg:    "Couldn't start the import with those server settings.",
		},
		{
			name:       "upstream failure",
			status:     http.StatusBadGateway,
			wantStatus: http.StatusBadGateway,
			wantCode:   "bad_gateway",
			wantMsg:    "The source server couldn't complete the import right now. Please try again.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotCode, gotMsg := historyImportUpstreamError(tt.status)
			if gotStatus != tt.wantStatus || gotCode != tt.wantCode || gotMsg != tt.wantMsg {
				t.Fatalf("got (%d, %q, %q), want (%d, %q, %q)", gotStatus, gotCode, gotMsg, tt.wantStatus, tt.wantCode, tt.wantMsg)
			}
		})
	}
}

func TestHistoryImportDurableAdmissionErrorsDoNotExposeCauses(t *testing.T) {
	err := errors.Join(historyimport.ErrPersonalAdmissionUncertain, errors.New("private credential detail"))
	out := historyImportAPIError(err)
	if out.Status != http.StatusServiceUnavailable || out.Message != historyimport.ErrPersonalAdmissionUncertain.Error() || !errors.Is(out, historyimport.ErrPersonalAdmissionUncertain) {
		t.Fatalf("unsafe uncertain admission mapping: %+v", out)
	}
}
