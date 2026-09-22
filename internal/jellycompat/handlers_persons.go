package jellycompat

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Silo-Server/silo-server/internal/catalog"
	"github.com/Silo-Server/silo-server/internal/models"
)

type personSearchSource interface {
	SearchVisible(context.Context, string, bool, int, int, catalog.AccessFilter, bool) ([]models.Person, int, error)
}

// PersonsHandler serves the Jellyfin /Persons endpoints.
type PersonsHandler struct {
	personRepo personSearchSource
	content    ContentService
	codec      *ResourceIDCodec
	images     *ImageCache
	serverID   string
	imageTags  *imageTagSigner
}

// NewPersonsHandler creates a new persons handler.
func NewPersonsHandler(personRepo *catalog.PersonRepository, content ContentService, codec *ResourceIDCodec, images *ImageCache, serverID, imageTagSecret string) *PersonsHandler {
	return &PersonsHandler{
		personRepo: personRepo,
		content:    content,
		codec:      codec,
		images:     images,
		serverID:   serverID,
		imageTags:  newImageTagSigner(imageTagSecret),
	}
}

// HandleGetPersons serves GET /Persons.
func (h *PersonsHandler) HandleGetPersons(w http.ResponseWriter, r *http.Request) {
	session := SessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return
	}

	q := newCaseInsensitiveQuery(r.URL.Query())
	searchTerm := strings.TrimSpace(q.Get("SearchTerm"))
	// Person queries use PostgreSQL directly and cap the returned page. The
	// optional SearchTerm accepts short names; an empty term lists visible people.
	limit := clampAuxSearchLimit(parsePositiveInt(q.Get("Limit"), auxSearchMaxResults))

	filter := catalog.AccessFilter{AllowedLibraryIDs: []int{}}
	if service, ok := h.content.(*directContentService); ok {
		filter = service.resolveFilter(r.Context(), session)
	}
	offset := parsePositiveInt(q.Get("StartIndex"), 0)
	includeTotal := parseBool(q.Get("EnableTotalRecordCount"), true)
	people, total, err := h.personRepo.SearchVisible(r.Context(), searchTerm, false, limit, offset, filter, includeTotal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}

	items := make([]baseItemDTO, 0, len(people))
	for _, p := range people {
		items = append(items, h.personToDTO(p))
	}

	writeJSON(w, http.StatusOK, queryResultDTO{
		Items:            items,
		TotalRecordCount: total,
		StartIndex:       offset,
	})
}

// HandleGetPerson serves GET /Persons/{name}.
func (h *PersonsHandler) HandleGetPerson(w http.ResponseWriter, r *http.Request) {
	session := SessionFromContext(r.Context())
	if session == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing authentication token")
		return
	}

	name := chi.URLParam(r, "name")
	filter := catalog.AccessFilter{AllowedLibraryIDs: []int{}}
	if service, ok := h.content.(*directContentService); ok {
		filter = service.resolveFilter(r.Context(), session)
	}
	people, _, err := h.personRepo.SearchVisible(r.Context(), name, true, 1, 0, filter, false)
	if err != nil {
		writeCompatUpstreamError(w, err)
		return
	}
	if len(people) == 0 {
		writeError(w, http.StatusNotFound, "NotFound", "Person not found")
		return
	}
	person := &people[0]
	writeJSON(w, http.StatusOK, h.personToDTO(*person))
}

// personToDTO maps a person returned by a visibility-filtered search, so it may
// carry a signed photo tag.
func (h *PersonsHandler) personToDTO(p models.Person) baseItemDTO {
	routeID := h.codec.EncodeIntID(EncodedIDPerson, p.ID)
	dto := baseItemDTO{
		ID:       routeID,
		Name:     p.Name,
		Type:     "Person",
		ServerID: h.serverID,
		Overview: p.Bio,
	}
	if p.PhotoPath != "" && p.PhotoPath != "-" {
		dto.ImageTags = map[string]string{compatImagePrimary: personPrimaryImageTag(h.imageTags, routeID, p.PhotoPath, p.PhotoThumbhash)}
	}
	return dto
}
