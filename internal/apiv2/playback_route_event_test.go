package apiv2

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
)

const playbackTestEvent = "55555555-5555-4555-8555-555555555555"

func playbackRouteEventFixture() map[string]any {
	return map[string]any{
		"installation_id":     playbackTestInstallation,
		"event_id":            playbackTestEvent,
		"protocol_version":    3,
		"playback_attempt_id": "attempt-0123456789",
		"session_id":          "11111111-1111-4111-8111-111111111111",
		"plan_id":             "plan-0123456789",
		"event":               "first_frame",
		"diagnostics":         map[string]string{"decoder_name": "synthetic", "unlisted": "dropped later by the service"},
	}
}

func TestPlaybackV2RouteEventIsAcceptedAndIdentified(t *testing.T) {
	deps, _ := catalogDeps(t)
	fake := &fakePlaybackService{}
	deps.Playback = fake
	h := newTestHandler(t, deps)
	rec := do(t, h, http.MethodPost, Prefix+"/playback/route-events", playbackJSON(t, playbackRouteEventFixture()), viewerHeaders())
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), `"event_id":"`+playbackTestEvent+`"`) || !strings.Contains(rec.Body.String(), `"outcome":"accepted"`) {
		t.Fatalf("route event: %d %q", rec.Code, rec.Body.String())
	}
	if fake.calls != 1 || fake.event.EventID != playbackTestEvent || fake.event.Event.Event != "first_frame" || fake.event.Event.SessionID != "11111111-1111-4111-8111-111111111111" || fake.event.Event.Diagnostics["decoder_name"] != "synthetic" || fake.caller.InstallationID != playbackTestInstallation {
		t.Fatalf("service call: %+v %+v", fake.caller, fake.event)
	}
	for name, mutate := range map[string]func(map[string]any){
		"event name":      func(b map[string]any) { b["event"] = "guessed" },
		"event id":        func(b map[string]any) { b["event_id"] = "not-a-uuid" },
		"missing id":      func(b map[string]any) { delete(b, "event_id") },
		"short attempt":   func(b map[string]any) { b["playback_attempt_id"] = "x" },
		"unknown field":   func(b map[string]any) { b["unexpected"] = 1 },
		"long class":      func(b map[string]any) { b["failure_classification"] = strings.Repeat("x", 65) },
		"too many quirks": func(b map[string]any) { b["applied_quirk_ids"] = make([]string, 17) },
	} {
		t.Run(name, func(t *testing.T) {
			calls := fake.calls
			body := playbackRouteEventFixture()
			mutate(body)
			rec := do(t, h, http.MethodPost, Prefix+"/playback/route-events", playbackJSON(t, body), viewerHeaders())
			if rec.Code != 422 || fake.calls != calls {
				t.Fatalf("validation: %d %s calls=%d", rec.Code, rec.Body.String(), fake.calls-calls)
			}
		})
	}
	fake.err = &handlers.PlaybackOperationError{Status: 429, Code: "event_rate_limited", Message: "drop"}
	requireProblem(t, do(t, h, http.MethodPost, Prefix+"/playback/route-events", playbackJSON(t, playbackRouteEventFixture()), viewerHeaders()), TypeRateLimited)
	fake.err = &handlers.PlaybackOperationError{Status: 403, Code: "forbidden", Message: "not yours"}
	requireProblem(t, do(t, h, http.MethodPost, Prefix+"/playback/route-events", playbackJSON(t, playbackRouteEventFixture()), viewerHeaders()), TypePermissionDenied)
	fake.err = nil
	requireProblem(t, do(t, h, http.MethodPost, Prefix+"/playback/route-events", playbackJSON(t, playbackRouteEventFixture()), nil), TypeAuthenticationRequired)
}

func playbackRouteEventFixtureCases() []fixtureCase {
	event := map[string]any{"installation_id": playbackTestInstallation, "event_id": playbackTestEvent, "protocol_version": 3, "playback_attempt_id": "attempt-0123456789", "session_id": "11111111-1111-4111-8111-111111111111", "event": "first_frame", "diagnostics": map[string]string{"decoder_name": "synthetic"}}
	return []fixtureCase{
		{name: "playback_route_event_accepted", operationID: "reportPlaybackRouteEvent", method: http.MethodPost, path: Prefix + "/playback/route-events", body: fixturePlaybackJSON(event), schema: "#/components/schemas/PlaybackRouteEventReceipt", scenario: "A diagnostic route event with a client-minted id is queued and acknowledged by that id.", status: 202, headers: viewerHeaders(), assertHeaders: []string{"Content-Type", "Cache-Control"}},
	}
}
