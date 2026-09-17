package apiv2

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

const (
	assetKindFile = "file"
)

const (
	assetKindArtwork  = "artwork"
	assetKindSubtitle = "subtitle"
	referenceField    = "ref"
)

type DownloadDeliveryService interface {
	ServeDownloadFile(http.ResponseWriter, *http.Request, string, bool) error
	ServeDownloadArtwork(http.ResponseWriter, *http.Request, string, string) error
	ServeDownloadSubtitle(http.ResponseWriter, *http.Request, string, string) error
}

func registerDownloadDelivery(reg *Registry) {
	type route struct {
		method, path, id, kind string
		proxy                  bool
	}
	routes := []route{
		{http.MethodGet, "/downloads/{id}/file", "downloadFile", assetKindFile, false},
		{http.MethodHead, "/downloads/{id}/file", "headDownloadFile", assetKindFile, false},
		{http.MethodGet, "/downloads/{id}/file-proxy", "downloadFileViaProxy", assetKindFile, true},
		{http.MethodHead, "/downloads/{id}/file-proxy", "headDownloadFileViaProxy", assetKindFile, true},
		{http.MethodGet, "/downloads/{id}/artwork/{kind}", "getDownloadArtwork", assetKindArtwork, false},
		{http.MethodGet, "/downloads/{id}/subtitles/{ref}", "getDownloadSubtitle", assetKindSubtitle, false},
	}
	for _, route := range routes {
		op := humaOp(route.method, Prefix+route.path, route.id, "downloads", "Stream an authorized download file or offline asset using the existing delivery service.")
		op.Parameters = []*huma.Param{{Name: "id", In: paramInPath, Required: true, Schema: &huma.Schema{Type: huma.TypeString, MinLength: new(1)}}}
		if route.kind != assetKindFile {
			name := kindField
			if route.kind == assetKindSubtitle {
				name = referenceField
			}
			op.Parameters = append(op.Parameters, &huma.Param{Name: name, In: paramInPath, Required: true, Schema: &huma.Schema{Type: huma.TypeString, MinLength: new(1)}})
		}
		op.Parameters = append(op.Parameters, &huma.Param{Name: "X-Silo-Device-Id", In: paramInHeader, Required: route.kind != assetKindFile, Schema: &huma.Schema{Type: huma.TypeString, MaxLength: new(128)}})
		headers := map[string]*huma.Param{}
		for _, name := range []string{adminSubtitleLengthHeader, directDisposition, adminSubtitleCacheHeader, directContentRange, directAcceptRanges, "Last-Modified", etagField, jobLocationHeader} {
			headers[name] = &huma.Param{Schema: &huma.Schema{Type: huma.TypeString}}
		}
		media := map[string]*huma.MediaType{}
		if route.method == http.MethodGet {
			types := []string{directMediaBinary}
			switch route.kind {
			case assetKindFile:
				types = append(types, "video/*", "audio/*", "multipart/byteranges")
			case assetKindArtwork:
				types = []string{"image/*"}
			case assetKindSubtitle:
				types = []string{docsTextMediaType, "text/vtt", "application/x-subrip", "text/x-ssa", "text/x-ass", "application/ttml+xml", directMediaBinary}
			}
			for _, name := range types {
				media[name] = &huma.MediaType{Schema: &huma.Schema{Type: huma.TypeString, Format: artworkBinaryFormat}}
			}
		}
		problem := func(description string) *huma.Response {
			return &huma.Response{Description: description, Content: map[string]*huma.MediaType{problemContentType: {Schema: reg.api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[Problem](), true, "")}}}
		}
		op.Responses = map[string]*huma.Response{"200": {Description: "Authorized bytes; HEAD returns only file metadata.", Content: media, Headers: headers}, "400": problem("Invalid subtitle reference."), "422": problem("Invalid download identity or device scope."), "404": problem("Download or asset not found in this scope."), "409": problem("The download is not active.")}
		if route.kind == assetKindFile {
			for _, name := range []string{artworkRangeHeader, directIfRange, ifMatchField, ifNoneMatchField, "If-Modified-Since", directIfUnmodified} {
				op.Parameters = append(op.Parameters, &huma.Param{Name: name, In: paramInHeader, Schema: &huma.Schema{Type: huma.TypeString}})
			}
			op.Responses["206"] = &huma.Response{Description: "Single or multipart byte range.", Content: media, Headers: headers}
			op.Responses["304"] = &huma.Response{Description: "Representation not modified.", Headers: headers}
			op.Responses["412"] = &huma.Response{Description: "File precondition failed.", Headers: headers}
			op.Responses["416"] = &huma.Response{Description: "Range not satisfiable.", Content: map[string]*huma.MediaType{docsTextMediaType: {Schema: &huma.Schema{Type: huma.TypeString}}}, Headers: headers}
		}
		if route.proxy {
			op.Responses["307"] = &huma.Response{Description: "Authorized temporary proxy location; preserve the original method and range headers.", Headers: headers}
			if route.method == http.MethodGet {
				op.Responses["307"].Content = map[string]*huma.MediaType{docsHTMLMediaType: {Schema: &huma.Schema{Type: huma.TypeString}}}
			}
		}
		RegisterRaw(reg, RawOperation{Operation: Operation{Operation: op, Class: ClassProfileScoped, ServiceBacked: true}, Protocol: "managed-download-" + route.kind, Reason: "Offline media and assets are binary streams; file routes preserve HTTP range, conditional and optional proxy semantics."}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reg.serveDownloadDelivery(w, r, route.kind, route.proxy)
		}))
	}
}
func (reg *Registry) serveDownloadDelivery(w http.ResponseWriter, r *http.Request, kind string, proxy bool) {
	if reg.deps.DownloadDelivery == nil {
		writeProblem(w, r, unavailable("download delivery"))
		return
	}
	id := chi.URLParam(r, "id")
	device := strings.TrimSpace(r.Header.Get("X-Silo-Device-Id"))
	if id == "" || len(device) > 128 || (kind != assetKindFile && device == "") {
		writeProblem(w, r, NewProblem(TypeValidationFailed, "A download identity and valid device scope are required."))
		return
	}
	writer := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
	var err error
	switch kind {
	case assetKindFile:
		err = reg.deps.DownloadDelivery.ServeDownloadFile(writer, r, id, proxy)
	case assetKindArtwork:
		err = reg.deps.DownloadDelivery.ServeDownloadArtwork(writer, r, id, chi.URLParam(r, kindField))
	case assetKindSubtitle:
		err = reg.deps.DownloadDelivery.ServeDownloadSubtitle(writer, r, id, chi.URLParam(r, referenceField))
	}
	if err == nil || writer.Status() != 0 {
		return
	}
	// Upstream assets can fail after setting binary headers but before writing.
	// Never append JSON after committed bytes or retain their advertised length.
	for _, name := range []string{directContentType, adminSubtitleLengthHeader, directContentRange, directDisposition, etagField, adminSubtitleCacheHeader} {
		w.Header().Del(name)
	}
	writeProblem(w, r, downloadProblem(err))
}
