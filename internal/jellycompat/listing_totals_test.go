package jellycompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Silo-Server/silo-server/internal/catalog"
	"github.com/Silo-Server/silo-server/internal/config"
	"github.com/Silo-Server/silo-server/internal/models"
)

type personTotalsSource struct {
	includeTotal bool
	limit        int
	offset       int
}

func (s *personTotalsSource) SearchVisible(_ context.Context, _ string, _ bool, limit, offset int, _ catalog.AccessFilter, includeTotal bool) ([]models.Person, int, error) {
	s.includeTotal, s.limit, s.offset = includeTotal, limit, offset
	total := 0
	if includeTotal {
		total = 5
	}
	return []models.Person{{ID: 1, Name: "Visible Person"}}, total, nil
}

func TestListingTotalRecordCountControls(t *testing.T) {
	for _, tc := range []struct {
		name, query  string
		includeTotal bool
	}{
		{name: "default", includeTotal: true},
		{name: "enabled", query: "&EnableTotalRecordCount=true", includeTotal: true},
		{name: "disabled", query: "&EnableTotalRecordCount=false"},
		{name: "case insensitive", query: "&enabletotalrecordcount=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			codec := NewResourceIDCodec()
			upcoming := &upcomingContractRepo{}
			items := &ItemsHandler{episodeRepo: upcoming, codec: codec, mapper: newMapper(codec, &config.Config{}), userData: &mockUserDataService{}}
			people := &personTotalsSource{}
			persons := &PersonsHandler{personRepo: people, codec: codec}
			for _, endpoint := range []struct {
				path    string
				handler http.HandlerFunc
			}{
				{path: "/Shows/Upcoming", handler: items.HandleUpcoming},
				{path: "/Persons", handler: persons.HandleGetPersons},
			} {
				t.Run(endpoint.path, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, endpoint.path+"?StartIndex=2&Limit=1"+tc.query, nil)
					req = req.WithContext(context.WithValue(t.Context(), compatSessionKey, collectionsTestSession()))
					rec := httptest.NewRecorder()
					endpoint.handler(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("response: %d %s", rec.Code, rec.Body.String())
					}
					var result queryResultDTO
					if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					wantTotal := 0
					if tc.includeTotal {
						wantTotal = 5
					}
					if result.TotalRecordCount != wantTotal || result.StartIndex != 2 || len(result.Items) != 1 || result.Items[0].ID == "" {
						t.Fatalf("total control changed the selected page: %+v", result)
					}
				})
			}
			if upcoming.includeTotal != tc.includeTotal || people.includeTotal != tc.includeTotal || upcoming.limit != 1 || upcoming.offset != 2 || people.limit != 1 || people.offset != 2 {
				t.Fatalf("catalog query controls: upcoming=%+v people=%+v", upcoming, people)
			}
		})
	}
}
