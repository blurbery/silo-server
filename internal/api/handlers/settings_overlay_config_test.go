package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Silo-Server/silo-server/internal/config"
)

func readOverlayConfig(t *testing.T, handler *SettingsHandler) overlayConfigResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.HandleGetOverlayConfig(
		recorder,
		httptest.NewRequest(http.MethodGet, "/settings/overlay-config", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response overlayConfigResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func TestGetOverlayConfigIncludesDefaultWebWatchedIndicator(t *testing.T) {
	response := readOverlayConfig(t, NewSettingsHandler(nil))
	if !response.Enabled {
		t.Fatal("overlays enabled = false, want true")
	}
	if response.WatchedIndicator != config.DefaultWebWatchedIndicatorStyle {
		t.Fatalf(
			"watched indicator = %q, want %q",
			response.WatchedIndicator,
			config.DefaultWebWatchedIndicatorStyle,
		)
	}
}

func TestGetOverlayConfigIncludesConfiguredWebWatchedIndicator(t *testing.T) {
	handler := NewSettingsHandler(nil)
	handler.SetServerSettings(&fakeServerSettingsStore{values: map[string]string{
		"overlays.enabled":                   "false",
		"defaults.card_overlays":             `{"preset":"classic"}`,
		config.WebWatchedIndicatorSettingKey: "eye",
	}})

	response := readOverlayConfig(t, handler)
	if response.Enabled {
		t.Fatal("overlays enabled = true, want false")
	}
	if response.Defaults != `{"preset":"classic"}` {
		t.Fatalf("defaults = %q", response.Defaults)
	}
	if response.WatchedIndicator != "eye" {
		t.Fatalf("watched indicator = %q, want eye", response.WatchedIndicator)
	}
}

func TestGetOverlayConfigFallsBackFromInvalidWebWatchedIndicator(t *testing.T) {
	handler := NewSettingsHandler(nil)
	handler.SetServerSettings(&fakeServerSettingsStore{values: map[string]string{
		config.WebWatchedIndicatorSettingKey: "invalid",
	}})

	response := readOverlayConfig(t, handler)
	if response.WatchedIndicator != config.DefaultWebWatchedIndicatorStyle {
		t.Fatalf(
			"watched indicator = %q, want fallback %q",
			response.WatchedIndicator,
			config.DefaultWebWatchedIndicatorStyle,
		)
	}
}
