package main

import (
	"context"
	"log/slog"

	"github.com/Silo-Server/silo-server/internal/metadata"
	"github.com/Silo-Server/silo-server/internal/plugins"
)

// Fork carry: a metadata plugin that startup installs by default joins every
// library's provider chain, as one uploaded through the admin API does (see
// PluginHandler.syncMetadataProviders). Without it a default plugin added to
// an existing server installs but sits in no chain, so no library uses it.

type pluginCapabilityLister interface {
	ListCapabilities(ctx context.Context, installationID int) ([]*plugins.Capability, error)
}

type providerChainAppender interface {
	AppendProviderToAllChains(ctx context.Context, pluginInstallationID int, capabilityID string, placement func(contentLevel string) metadata.SeedPlacement) error
}

// appendInstalledMetadataProviders returns the auto-updater's install hook. Each
// metadata provider capability goes into every stored chain at the place its
// manifest's default_priority gives it; placement reads that through seed.
func appendInstalledMetadataProviders(
	capabilities pluginCapabilityLister,
	chains providerChainAppender,
	seed func(ctx context.Context, installationID int, capabilityID, level string) metadata.SeedPlacement,
) func(context.Context, *plugins.Installation) {
	return func(ctx context.Context, installation *plugins.Installation) {
		caps, err := capabilities.ListCapabilities(ctx, installation.ID)
		if err != nil {
			slog.WarnContext(ctx, "listing capabilities of an auto-installed plugin", "component", "plugins",
				"plugin_id", installation.PluginID, "installation_id", installation.ID, "error", err)
			return
		}
		for _, capability := range caps {
			if capability == nil || capability.Type != "metadata_provider.v1" {
				continue
			}
			capabilityID := capability.ID
			if err := chains.AppendProviderToAllChains(ctx, installation.ID, capabilityID, func(level string) metadata.SeedPlacement {
				return seed(ctx, installation.ID, capabilityID, level)
			}); err != nil {
				slog.WarnContext(ctx, "failed to append an auto-installed provider to library chains", "component", "plugins",
					"plugin_id", installation.PluginID, "capability_id", capabilityID, "error", err)
				continue
			}
			slog.InfoContext(ctx, "added an auto-installed metadata provider to library chains", "component", "plugins",
				"plugin_id", installation.PluginID, "capability_id", capabilityID)
		}
	}
}
