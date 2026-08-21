package metadata

import (
	"context"
	"errors"
	"testing"
	"time"
)

type seasonImageProvider struct {
	slug     string
	seasons  []SeasonResult
	err      error
	requests []SeasonsRequest
}

type itemImageProvider struct {
	slug   string
	images []RemoteImage
}

func (p *itemImageProvider) Slug() string       { return p.slug }
func (p *itemImageProvider) Name() string       { return p.slug }
func (p *itemImageProvider) ForTypes() []string { return []string{"series"} }

func (p *itemImageProvider) GetImages(context.Context, ImageRequest) ([]RemoteImage, error) {
	return p.images, nil
}

func (p *seasonImageProvider) Slug() string       { return p.slug }
func (p *seasonImageProvider) Name() string       { return p.slug }
func (p *seasonImageProvider) ForTypes() []string { return []string{"series"} }

func (p *seasonImageProvider) GetSeasons(_ context.Context, req SeasonsRequest) ([]SeasonResult, error) {
	p.requests = append(p.requests, req)
	return p.seasons, p.err
}

func (p *seasonImageProvider) GetEpisodes(context.Context, EpisodesRequest) ([]EpisodeResult, error) {
	return nil, nil
}

func TestFetchSeasonImagesReturnsOnlyRequestedSeason(t *testing.T) {
	tests := []struct {
		name         string
		seasonNumber int
		poster       string
	}{
		{name: "numbered season", seasonNumber: 2, poster: "tmdb://season-2.jpg"},
		{name: "specials", seasonNumber: 0, poster: "tmdb://specials.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &seasonImageProvider{
				slug: "tmdb",
				seasons: []SeasonResult{
					{SeasonNumber: 1, PosterPath: "tmdb://season-1.jpg"},
					{SeasonNumber: tt.seasonNumber, PosterPath: tt.poster},
					{SeasonNumber: tt.seasonNumber, PosterPath: "tmdb://duplicate-season-result.jpg"},
				},
			}
			service := &MetadataService{chainCache: map[string]chainCacheEntry{
				"42:season": {providers: []Provider{provider}, expiresAt: time.Now().Add(time.Hour)},
			}}

			providerIDs := map[string]string{"tmdb": "123"}
			images, providerErrors, err := service.FetchSeasonImages(
				context.Background(), providerIDs, "en", 42, tt.seasonNumber,
			)
			if err != nil {
				t.Fatalf("FetchSeasonImages() error = %v", err)
			}
			if len(providerErrors) != 0 {
				t.Fatalf("provider errors = %v, want none", providerErrors)
			}
			if len(images) != 1 {
				t.Fatalf("images = %#v, want one exact-season poster", images)
			}
			if got := images[0]; got.ProviderID != "tmdb" || got.URL != tt.poster || got.Type != ImagePoster || got.Language != "" {
				t.Fatalf("image = %#v, want tmdb poster %q", got, tt.poster)
			}
			if len(provider.requests) != 1 {
				t.Fatalf("GetSeasons calls = %d, want 1", len(provider.requests))
			}
			gotReq := provider.requests[0]
			if gotReq.ContentType != "series" || gotReq.Language != "en" || gotReq.ProviderIDs["tmdb"] != "123" {
				t.Fatalf("GetSeasons request = %#v", gotReq)
			}
		})
	}
}

func TestFetchSeasonImagesKeepsProviderErrorsAndContinues(t *testing.T) {
	failing := &seasonImageProvider{slug: "tvdb", err: errors.New("provider unavailable")}
	working := &seasonImageProvider{slug: "tmdb", seasons: []SeasonResult{{
		SeasonNumber: 3,
		PosterPath:   "tmdb://season-3.jpg",
	}}}
	service := &MetadataService{chainCache: map[string]chainCacheEntry{
		"7:season": {providers: []Provider{failing, working}, expiresAt: time.Now().Add(time.Hour)},
	}}

	images, providerErrors, err := service.FetchSeasonImages(
		context.Background(), map[string]string{"tvdb": "456", "tmdb": "123"}, "en", 7, 3,
	)
	if err != nil {
		t.Fatalf("FetchSeasonImages() error = %v", err)
	}
	if len(images) != 1 || images[0].URL != "tmdb://season-3.jpg" {
		t.Fatalf("images = %#v, want working provider poster", images)
	}
	if providerErrors["tvdb"] != "provider unavailable" {
		t.Fatalf("provider errors = %v", providerErrors)
	}
}

func TestFetchItemImagesFiltersIllustratedLogoSources(t *testing.T) {
	tvdb := &itemImageProvider{slug: "tvdb", images: []RemoteImage{
		{ProviderID: "tvdb", URL: "tvdb://show-poster.jpg", Type: ImagePoster, Rating: 3},
		{ProviderID: "tvdb", URL: "tvdb://illustrated-clear-art.png", Type: ImageLogo, Rating: 10},
	}}
	tmdb := &itemImageProvider{slug: "tmdb", images: []RemoteImage{
		{ProviderID: "tmdb", URL: "tmdb://lower-rated-wordmark.png", Type: ImageLogo, Rating: 5},
		{ProviderID: "tmdb", URL: "tmdb://backdrop.jpg", Type: ImageBackdrop, Rating: 7},
		{ProviderID: "tmdb", URL: "tmdb://top-rated-wordmark.png", Type: ImageLogo, Rating: 9},
	}}
	service := &MetadataService{chainCache: map[string]chainCacheEntry{
		"11:series": {providers: []Provider{tvdb, tmdb}, expiresAt: time.Now().Add(time.Hour)},
	}}

	images, providerErrors, err := service.FetchItemImages(
		context.Background(), map[string]string{"tvdb": "456", "tmdb": "123"}, "series", "en", 11,
	)
	if err != nil {
		t.Fatalf("FetchItemImages() error = %v", err)
	}
	if len(providerErrors) != 0 {
		t.Fatalf("provider errors = %v, want none", providerErrors)
	}
	if len(images) != 4 {
		t.Fatalf("images = %#v, want all non-logo art and TMDB wordmarks", images)
	}
	for _, image := range images {
		if image.ProviderID == "tvdb" && image.Type == ImageLogo {
			t.Fatalf("illustrated TVDB logo was not filtered: %#v", image)
		}
	}
	for i := 1; i < len(images); i++ {
		if images[i-1].Rating < images[i].Rating {
			t.Fatalf("images are not sorted by rating: %#v", images)
		}
	}
}
