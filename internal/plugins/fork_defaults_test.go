package plugins

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Fork carry: fanart.tv installs by default from the fork's own catalog.

// installingAutoUpdateInstaller reports the installation it created, which the
// shared fake leaves empty.
type installingAutoUpdateInstaller struct {
	*fakeAutoUpdateInstaller
	installed *Installation
}

func (f installingAutoUpdateInstaller) InstallBinary(ctx context.Context, req InstallBinaryRequest) (*InstallResult, error) {
	if _, err := f.fakeAutoUpdateInstaller.InstallBinary(ctx, req); err != nil {
		return nil, err
	}
	return &InstallResult{Installation: f.installed}, nil
}

func TestAutoUpdateInstallsFanartTVFromTheForkCatalogOnce(t *testing.T) {
	catalog := &fakeAutoUpdateCatalog{
		entries: []CatalogEntry{
			{RepositoryID: 7, SourceKind: RepositorySourceSilo, Manifest: &pluginv1.PluginManifest{PluginId: pluginIDTMDB, Version: "1.0.0"}},
			{RepositoryID: 8, SourceKind: RepositorySourceSilo, Manifest: &pluginv1.PluginManifest{PluginId: pluginIDFanartTV, Version: "0.2.0"}},
		},
		resolved: &ResolvedCatalogInstall{RepositoryID: 8, ArchiveURL: "https://plugins.example.test/fanarttv", Checksum: "test-checksum"},
	}
	installations := &fakeAutoUpdateInstallations{list: []*Installation{
		{PluginID: pluginIDTMDB, Enabled: true},
		{PluginID: "silo.tvdb", Enabled: true},
		{PluginID: "silo.theintrodb", Enabled: true},
	}}
	installed := &Installation{ID: 11, PluginID: pluginIDFanartTV}
	installer := installingAutoUpdateInstaller{fakeAutoUpdateInstaller: &fakeAutoUpdateInstaller{}, installed: installed}
	service := NewAutoUpdateService(&fakeAutoUpdateRepositories{}, installations, catalog, installer, nil, nil, nil)
	var hooked []*Installation
	service.SetOnInstalled(func(_ context.Context, installation *Installation) {
		hooked = append(hooked, installation)
	})

	summary, err := service.Check(t.Context(), AutoUpdateOptions{AutoInstallDefaults: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.DefaultPluginsInstalled != 1 || len(catalog.resolveRequests) != 1 ||
		catalog.resolveRequests[0].PluginID != pluginIDFanartTV || catalog.resolveRequests[0].RepositoryID != 8 {
		t.Fatalf("installs = %d, resolved = %+v, want fanart.tv from the fork catalog", summary.DefaultPluginsInstalled, catalog.resolveRequests)
	}
	if len(hooked) != 1 || hooked[0] != installed {
		t.Fatalf("install hook ran for %v, want once for the new installation", hooked)
	}

	// Once installed, later startups leave it alone and do not re-run the hook,
	// so a chain an administrator edited keeps their edit.
	installations.list = append(installations.list, installed)
	if _, err := service.Check(t.Context(), AutoUpdateOptions{AutoInstallDefaults: true}); err != nil {
		t.Fatal(err)
	}
	if len(hooked) != 1 || len(catalog.resolveRequests) != 1 {
		t.Fatalf("second startup: hook ran %d times, resolved %+v", len(hooked), catalog.resolveRequests)
	}
}

func TestForkCatalogIsAManagedSiloSource(t *testing.T) {
	var fork *managedRepositoryDefinition
	for i := range managedRepositoryDefinitions {
		if managedRepositoryDefinitions[i].Key == ForkRepositoryManagedKey {
			fork = &managedRepositoryDefinitions[i]
		}
	}
	if fork == nil {
		t.Fatal("the fork catalog is not a managed repository")
	}
	if fork.URL != ForkRepositoryURL || fork.SourceKind != RepositorySourceSilo {
		t.Fatalf("fork catalog = %+v, want %s as a Silo source", fork, ForkRepositoryURL)
	}
}

// recordingCatalogExecutor answers the existence check and records each
// managed repository's enabled flag.
type recordingCatalogExecutor struct {
	enabled map[string]bool
}

type existsRow struct{}

func (existsRow) Scan(dest ...any) error {
	*(dest[0].(*bool)) = true
	return nil
}

func (e *recordingCatalogExecutor) QueryRow(context.Context, string, ...any) pgx.Row {
	return existsRow{}
}

func (e *recordingCatalogExecutor) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	e.enabled[args[3].(string)] = args[2].(bool)
	return pgconn.CommandTag{}, nil
}

func TestReconcileKeepsSiloSourcesEnabledWithoutCommunityPlugins(t *testing.T) {
	executor := &recordingCatalogExecutor{enabled: map[string]bool{}}
	if _, err := reconcileManagedRepositories(t.Context(), executor, false); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		OfficialRepositoryManagedKey:          true,
		ApprovedCommunityRepositoryManagedKey: false,
		ForkRepositoryManagedKey:              true,
	}
	for key, enabled := range want {
		if executor.enabled[key] != enabled {
			t.Errorf("%s enabled = %v, want %v (all: %v)", key, executor.enabled[key], enabled, executor.enabled)
		}
	}
}
