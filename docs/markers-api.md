# Marker API

The v2 marker API reads effective file markers and updates manual markers. Each
operation applies the existing file, library, item, episode-parent, and extra-parent
access policy. Reads require authentication. Writes also require `marker_edit`
permission and pass the demo and viewer-access gates. Profile context remains
optional; supplying a profile applies its access restrictions.

| Method | Path | Operation |
| --- | --- | --- |
| GET | `/api/v2/markers/files/{file_id}` | `getFileMarkers` |
| GET | `/api/v2/markers/items/{item_id}` | `getItemMarkers` |
| PUT | `/api/v2/markers/files/{file_id}` | `setFileMarkers` |
| PUT | `/api/v2/markers/items/{item_id}` | `setItemMarkers` |
| DELETE | `/api/v2/markers/files/{file_id}/{segment}` | `clearFileMarkerSegment` |

Item routes select the item's first accessible file, using episode files before
content files. Every success returns HTTP 200 with a string `file_id` and four
objects: `intro`, `credits`, `recap`, and `preview`. A segment with no marker is an
empty object. Present boundaries are `start_seconds` and `end_seconds`, measured
in source-file seconds. Provenance fields (`source`, `provider`, `confidence`,
`algorithm`, `detected_at`) are omitted when absent. Timestamps use the shared v2
instant format.

PUT accepts a partial update: omit a segment to leave it unchanged, send `null`
to clear it, or send an object to set it. For example:

```json
{"intro":{"end_seconds":48.5},"credits":null}
```

This sets intro from zero to 48.5 seconds, clears credits, and preserves recap and
preview. Intro and recap default a missing start to zero and require an end.
Credits and preview require a start and default a missing end to the known file
duration. Individual boundaries cannot be null. Unknown fields, negative or
nonfinite boundaries, and an end at or before the start are rejected. Existing
duration validation retains its one-second tolerance; an unknown duration cannot
supply a default end.

All supplied segments are validated before the shared writer commits the mixed
set/clear update and its audit rows in one transaction. Audit identity comes from
authenticated claims. Failed validation or audit insertion rolls back the update.
After commit, the service reloads the effective markers, publishes a notification,
and may contribute newly set segments through the existing provider service.

Writes advertise `non_retryable`. Database no-op detection and contribution
claims do not guarantee exactly-once external effects: notifications may repeat,
and a provider can accept a contribution before its response or local receipt is
lost. A failed response can therefore follow a successful save. Read the current
markers before deciding whether another user-directed update is needed.

The legacy v1 adapter shares the manual writer path and retains its wire format.
Jellyfin does not expose these manual editing routes. Native marker call sites
were absent from the migration inventory; native playback lifecycle and delivery
migration are separate work. The generated OpenAPI document is the authoritative
v2 schema and error contract.
