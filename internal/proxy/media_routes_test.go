package proxy

import (
	"flag"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Silo-Server/silo-server/internal/nodeconfig"
	"github.com/Silo-Server/silo-server/internal/nodesessions"
	"github.com/Silo-Server/silo-server/internal/streamtelemetry"
)

var updateRouteManifest = flag.Bool("update-route-manifest", false, "update checked-in route manifest")

func TestMediaRouteManifest(t *testing.T) {
	declareProxyMediaRoutes()
	makeRouter := func() chi.Routes {
		return NewServer(nodeconfig.NewWatcher(nil, nil, nil, nodeconfig.BootstrapOverrides{}), nodesessions.NewTracker(nil, "", "", "")).Handler().(chi.Routes)
	}
	assertMediaManifest(t, []chi.Routes{makeRouter(), makeRouter()}, proxyMediaRoutes, "testdata/media_routes.txt")
}

func assertMediaManifest(t *testing.T, fixtures []chi.Routes, declared []streamtelemetry.MediaRoute, path string) {
	t.Helper()
	actual, err := streamtelemetry.BuildRouteManifest(fixtures, declared)
	if err != nil {
		t.Fatal(err)
	}
	if *updateRouteManifest {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(actual), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != actual {
		t.Fatalf("route manifest changed; inspect it and run go test . -update-route-manifest")
	}
	for _, route := range declared {
		if !route.Enrolled || route.Capture == nil {
			t.Fatalf("proxy route not fully enrolled: %s %s", route.Method, route.Pattern)
		}
	}
}
