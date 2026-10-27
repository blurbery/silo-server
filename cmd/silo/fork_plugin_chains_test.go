package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Silo-Server/silo-server/internal/metadata"
	"github.com/Silo-Server/silo-server/internal/plugins"
)

type fakeCapabilityLister struct {
	caps []*plugins.Capability
	err  error
}

func (f fakeCapabilityLister) ListCapabilities(context.Context, int) ([]*plugins.Capability, error) {
	return f.caps, f.err
}

type appendCall struct {
	installationID int
	capabilityID   string
	placement      metadata.SeedPlacement
}

type fakeChainAppender struct {
	calls []appendCall
}

func (f *fakeChainAppender) AppendProviderToAllChains(_ context.Context, installationID int, capabilityID string, placement func(string) metadata.SeedPlacement) error {
	f.calls = append(f.calls, appendCall{installationID, capabilityID, placement("movie")})
	return nil
}

func TestAppendInstalledMetadataProvidersAddsOnlyMetadataProviders(t *testing.T) {
	lister := fakeCapabilityLister{caps: []*plugins.Capability{
		{Type: "metadata_provider.v1", ID: "fanarttv"},
		{Type: "image_resolver.v1", ID: "fanarttv"},
		nil,
	}}
	chains := &fakeChainAppender{}
	var seeded []string
	hook := appendInstalledMetadataProviders(lister, chains, func(_ context.Context, installationID int, capabilityID, level string) metadata.SeedPlacement {
		seeded = append(seeded, capabilityID+"/"+level)
		return metadata.SeedPlacement{SupportsLevel: true, DefaultPriority: 5, DefaultEnabled: true}
	})

	hook(t.Context(), &plugins.Installation{ID: 11, PluginID: "blurbery.fanarttv"})

	if len(chains.calls) != 1 || chains.calls[0].installationID != 11 || chains.calls[0].capabilityID != "fanarttv" {
		t.Fatalf("appended %+v, want the one metadata provider of installation 11", chains.calls)
	}
	if chains.calls[0].placement.DefaultPriority != 5 || len(seeded) != 1 || seeded[0] != "fanarttv/movie" {
		t.Fatalf("placement = %+v from %v, want the manifest's default priority", chains.calls[0].placement, seeded)
	}
}

func TestAppendInstalledMetadataProvidersSurvivesAListingFailure(t *testing.T) {
	chains := &fakeChainAppender{}
	hook := appendInstalledMetadataProviders(fakeCapabilityLister{err: errors.New("database unavailable")}, chains,
		func(context.Context, int, string, string) metadata.SeedPlacement { return metadata.SeedPlacement{} })

	hook(t.Context(), &plugins.Installation{ID: 11, PluginID: "blurbery.fanarttv"})

	if len(chains.calls) != 0 {
		t.Fatalf("appended %+v after the capability listing failed", chains.calls)
	}
}
