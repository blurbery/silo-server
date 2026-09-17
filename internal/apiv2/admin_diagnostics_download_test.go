package apiv2

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	apimw "github.com/Silo-Server/silo-server/internal/api/middleware"
	"github.com/Silo-Server/silo-server/internal/diagnostics"
)

type diagnosticDownloadStub struct {
	calls int
	user  int
	id    string
	body  *diagnosticTestBody
	err   error
}
type diagnosticTestBody struct {
	io.Reader
	closed bool
}

func (b *diagnosticTestBody) Close() error { b.closed = true; return nil }
func (s *diagnosticDownloadStub) OpenAdminDiagnosticDownload(ctx context.Context, id string) (handlers.AdminDiagnosticDownload, error) {
	s.calls++
	s.id = id
	s.user = apimw.GetUserID(ctx)
	return handlers.AdminDiagnosticDownload{Filename: "report.tar.gz", Size: new(int64(6)), Body: s.body}, s.err
}
func TestAdminDiagnosticDownload(t *testing.T) {
	s := &diagnosticDownloadStub{body: &diagnosticTestBody{Reader: strings.NewReader("bundle")}}
	deps := requestDeps(fixtureRequests())
	deps.AdminDiagnosticDownloads = s
	h := NewHandler(deps)
	path := Prefix + "/admin/diagnostics/reports/report-1/download"
	if r := do(t, h, http.MethodGet, path, "", nil); r.Code != 401 || s.calls != 0 {
		t.Fatal(r.Code, s.calls)
	}
	r := do(t, h, http.MethodGet, path, "", with(actingRequestAdmin, "Range", "bytes=1-2"))
	if r.Code != 200 || r.Body.String() != "bundle" || !s.body.closed || s.calls != 1 || s.user == 0 || s.id != "report-1" {
		t.Fatal(r.Code, r.Body.String(), s)
	}
	for key, want := range map[string]string{"Content-Type": "application/gzip", "Content-Length": "6", "Accept-Ranges": "none", "Cache-Control": "no-store", "Content-Disposition": "attachment; filename=report.tar.gz"} {
		if got := r.Header().Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
func TestAdminDiagnosticDownloadProblems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"missing", diagnostics.ErrNotFound, 404}, {"pending", diagnostics.ErrReportNotReady, 409}, {"store", diagnostics.ErrReportStoreUnavailable, 503}, {"storage", diagnostics.ErrStorageUnavailable, 503}, {"internal", errors.New("private storage credential"), 500}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &diagnosticDownloadStub{err: tc.err}
			deps := requestDeps(fixtureRequests())
			deps.AdminDiagnosticDownloads = s
			r := do(t, NewHandler(deps), http.MethodGet, Prefix+"/admin/diagnostics/reports/report-1/download", "", actingRequestAdmin)
			if r.Code != tc.status || !strings.Contains(r.Header().Get("Content-Type"), "application/problem+json") || strings.Contains(r.Body.String(), "credential") {
				t.Fatal(r.Code, r.Body.String())
			}
		})
	}
}
