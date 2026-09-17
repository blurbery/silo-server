package apiv2

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	"github.com/Silo-Server/silo-server/internal/branding"
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

const (
	mediaTypePNG = "image/png"
	mediaTypeSVG = "image/svg+xml"
)

const (
	kindField = "kind"
)

type BrandingService interface {
	HasStorage() bool
	Load(context.Context) branding.Snapshot
	GetAsset(context.Context, branding.AssetKind) ([]byte, string, string, error)
}
type ThemeOverrideService interface {
	AdminCSS(context.Context) handlers.AdminCSSView
}
type BrandingConfiguration struct {
	ServerName       string `json:"server_name"`
	LoginSubtitle    string `json:"login_subtitle"`
	AccentColor      string `json:"accent_color,omitempty"`
	DefaultTheme     string `json:"default_theme,omitempty"`
	WordmarkURL      string `json:"wordmark_url,omitempty"`
	WordmarkLightURL string `json:"wordmark_light_url,omitempty"`
	MarkURL          string `json:"mark_url,omitempty"`
	MarkLightURL     string `json:"mark_light_url,omitempty"`
	FaviconURL       string `json:"favicon_url,omitempty"`
	LoginBgURL       string `json:"login_bg_url,omitempty"`
	StorageAvailable bool   `json:"storage_available"`
}
type BrandingOutput struct{ Body BrandingConfiguration }
type ThemeOverrides struct {
	Vars   string `json:"vars"`
	RawCSS string `json:"raw_css"`
}
type ThemeOverridesOutput struct{ Body ThemeOverrides }
type BrandingCapabilitiesOutput struct {
	Status       int
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	Body         BrandingCapabilitiesOutputBody
}

type BrandingCapabilitiesOutputBody struct {
	Capability
	BrandingAvailable  bool `json:"branding_available"`
	OverridesAvailable bool `json:"overrides_available"`
	StorageAvailable   bool `json:"storage_available"`
}

func brandingAssetURL(snap branding.Snapshot, kind branding.AssetKind) string {
	ref := snap.AssetRef(kind)
	if ref == "" {
		return ""
	}
	return Prefix + "/branding/assets/" + string(kind) + "?v=" + url.QueryEscape(ref)
}
func registerBranding(reg *Registry) {
	op := func(path, id, summary string) Operation {
		return Operation{Operation: humaOp(http.MethodGet, Prefix+path, id, "branding", summary), Class: ClassPublic, ServiceBacked: true}
	}
	Register(reg, op("/theme/capabilities", "getBrandingCapabilities", "Discover public branding and asset storage availability."), func(_ context.Context, _ *CapabilityInput) (*BrandingCapabilitiesOutput, error) {
		out := new(BrandingCapabilitiesOutput)
		out.Body.BrandingAvailable = reg.deps.Branding != nil
		out.Body.OverridesAvailable = reg.deps.ThemeOverrides != nil
		out.Body.StorageAvailable = reg.deps.Branding != nil && reg.deps.Branding.HasStorage()
		return out, nil
	})
	Register(reg, op("/theme/branding", "getBranding", "Read public pre-login branding."), func(ctx context.Context, _ *struct{}) (*BrandingOutput, error) {
		snap := branding.Snapshot{ServerName: branding.DefaultServerName, LoginSubtitle: branding.DefaultLoginSubtitle}
		storage := false
		if reg.deps.Branding != nil {
			snap = reg.deps.Branding.Load(ctx)
			storage = reg.deps.Branding.HasStorage()
		}
		return &BrandingOutput{Body: BrandingConfiguration{ServerName: snap.ServerName, LoginSubtitle: snap.LoginSubtitle, AccentColor: snap.AccentColor, DefaultTheme: snap.DefaultTheme, WordmarkURL: brandingAssetURL(snap, branding.KindWordmark), WordmarkLightURL: brandingAssetURL(snap, branding.KindWordmarkLight), MarkURL: brandingAssetURL(snap, branding.KindMark), MarkLightURL: brandingAssetURL(snap, branding.KindMarkLight), FaviconURL: brandingAssetURL(snap, branding.KindFavicon), LoginBgURL: brandingAssetURL(snap, branding.KindLoginBg), StorageAvailable: storage}}, nil
	})
	Register(reg, op("/theme/admin-css", "getThemeOverrides", "Read public pre-login theme overrides."), func(ctx context.Context, _ *struct{}) (*ThemeOverridesOutput, error) {
		view := handlers.AdminCSSView{}
		if reg.deps.ThemeOverrides != nil {
			view = reg.deps.ThemeOverrides.AdminCSS(ctx)
		}
		return &ThemeOverridesOutput{Body: ThemeOverrides{Vars: view.Vars, RawCSS: view.RawCSS}}, nil
	})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		id := "getBrandingAsset"
		if method == http.MethodHead {
			id = "headBrandingAsset"
		}
		raw := op("/branding/assets/{kind}", id, "Read a public branding image with content-version validation.")
		raw.Method = method
		raw.Parameters = []*huma.Param{
			{Name: kindField, In: paramInPath, Required: true, Schema: &huma.Schema{Type: schemaTypeString, Enum: []any{"wordmark", "wordmark_light", "mark", "mark_light", "favicon", "login_bg"}}},
			{Name: "v", In: artworkParamQuery, Schema: &huma.Schema{Type: schemaTypeString}, Description: "Content reference returned by branding discovery; a stale reference returns 404."},
			{Name: ifMatchField, In: paramInHeader, Schema: &huma.Schema{Type: schemaTypeString}},
			{Name: ifNoneMatchField, In: paramInHeader, Schema: &huma.Schema{Type: schemaTypeString}},
		}
		headers := map[string]*huma.Param{adminSubtitleLengthHeader: {Schema: &huma.Schema{Type: schemaTypeString}}, etagField: {Schema: &huma.Schema{Type: schemaTypeString}}, adminSubtitleCacheHeader: {Schema: &huma.Schema{Type: schemaTypeString}}, "Content-Security-Policy": {Schema: &huma.Schema{Type: schemaTypeString}}, "X-Content-Type-Options": {Schema: &huma.Schema{Type: schemaTypeString}}}
		content := map[string]*huma.MediaType{}
		if method == http.MethodGet {
			for _, media := range []string{mediaTypePNG, "image/webp", mediaTypeSVG, "image/x-icon", directMediaBinary} {
				content[media] = &huma.MediaType{Schema: &huma.Schema{Type: schemaTypeString, Format: artworkBinaryFormat}}
			}
		}
		raw.Responses = map[string]*huma.Response{
			"200": {Description: "Branding image bytes (no body for HEAD)", Headers: headers, Content: content},
			"304": {Description: "Image unchanged", Headers: headers},
			"404": {Description: "Unknown, absent, or stale asset"},
			"412": {Description: "Image precondition failed"},
			"500": {Description: "Asset could not be read"},
		}
		RegisterRaw(reg, RawOperation{Operation: raw, Protocol: "branding-image", Reason: "Image bytes and HEAD/conditional HTTP semantics bypass JSON encoding."}, http.HandlerFunc(reg.serveBrandingAsset))
	}
}
func (reg *Registry) serveBrandingAsset(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, kindField)
	if !branding.IsValidKind(kind) {
		writeProblem(w, r, NewProblem(TypeNotFound, "Unknown branding asset."))
		return
	}
	if reg.deps.Branding == nil {
		writeProblem(w, r, unavailable("branding assets"))
		return
	}
	data, media, ref, err := reg.deps.Branding.GetAsset(r.Context(), branding.AssetKind(kind))
	switch {
	case errors.Is(err, branding.ErrAssetNotConfigured):
		writeProblem(w, r, NewProblem(TypeNotFound, "No custom branding asset configured."))
		return
	case errors.Is(err, branding.ErrStorageUnavailable):
		writeProblem(w, r, unavailable("branding assets"))
		return
	case err != nil:
		writeProblem(w, r, serviceProblem(err))
		return
	}
	version := r.URL.Query().Get("v")
	if version != "" && version != ref {
		writeProblem(w, r, NewProblem(TypeNotFound, "This branding asset version is no longer current."))
		return
	}
	tag := EntityTag{Opaque: ref}
	// Immutable caching is safe only for an exact content-version request.
	w.Header().Set(adminSubtitleCacheHeader, "public, no-cache")
	if version != "" {
		w.Header().Set(adminSubtitleCacheHeader, "public, max-age=31536000, immutable")
	}
	w.Header().Set(etagField, tag.String())
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", branding.AssetContentSecurityPolicy)
	if matched, p := EvaluateReadPreconditions(r.Header.Get(ifMatchField), r.Header.Get(ifNoneMatchField), tag); p != nil {
		w.Header().Set(adminSubtitleCacheHeader, "no-store")
		writeProblem(w, r, p)
		return
	} else if matched {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set(adminSubtitleLengthHeader, strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (c BrandingCapabilitiesOutputBody) capabilityState() string {
	return configuredCapabilityState(c.BrandingAvailable)
}
