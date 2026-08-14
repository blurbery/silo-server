# Direct Stream Resumption

Protocol v3 advertises `direct_stream_resume_v1` for playback plans whose
delivery is `original_http`. The capability formalizes resumption of the
original file by issuing sequential authorized HTTP requests. It does not
weaken authorization or playback-session ownership.

Integrated playback URLs carry a signed `st` transport capability bound to the
playback session, user, profile, media file, and delivery method. Byte-delivery
routes accept that capability or fall back to ordinary account authentication.
The handler compares capability claims with the live session before serving, so
an older URL cannot cross an in-place replan boundary. Viewer-scope policy is
evaluated when the plan or replacement is admitted; it is not re-evaluated for
every media range. This keeps native media loaders working across access-token
refreshes without turning the stream capability into general API access.

Original-file responses follow the HTTP byte-range contract:

- `Accept-Ranges: bytes` advertises byte addressing.
- A satisfiable `Range` request returns `206 Partial Content` with the selected
  interval in `Content-Range`.
- Open-ended and suffix byte ranges are supported.
- A range starting at or past end of file, or another invalid range, returns
  `416 Requested Range Not Satisfiable` with `Content-Range: bytes */<size>`.
- `HEAD` returns the same representation headers as `GET` without a body.

On Linux, macOS, and Windows, each response carries a strong, opaque `ETag`
derived from durable filesystem identity plus content-relevant timestamps and
size. On Linux, permission, ownership, ACL, and xattr updates do not invalidate
the validator because they do not change the served bytes. A normal atomic
replacement at the same path changes the device/inode identity even when its
size and modification time are preserved; ordinary in-place writes change
modification time or size. A same-inode, same-size rewrite that deliberately
restores the exact modification time cannot be distinguished without hashing
the media file and is outside this stat-based contract. Platforms that cannot
expose a durable filesystem revision omit the validator instead of hashing an
entire media file before each request. On those platforms an ETag-based
`If-Range` request cannot match and safely falls back to a full `200 OK`
response.

A client resuming a transfer sends both `Range` and `If-Range` with the
validator. When it still matches, the server returns the requested `206`
response. When the entity changed, the server ignores the range and returns the
entire current entity as `200 OK`, preventing bytes from different revisions
from being combined. `If-None-Match` uses the same validator for ordinary
conditional requests.

For a bounded chunk read, a client can instead send `Range` and `If-Match`.
When the validator matches, the server returns the requested `206` interval.
When it does not match, the server returns `412 Precondition Failed` without a
body. This contract avoids materializing a full entity when a bounded read's
validator is stale. It does not change `If-Range`: an `If-Range` mismatch still
ignores the range and returns the full current entity as `200 OK`, as required
by the HTTP range contract.

Direct-stream completion logs record whether `If-Match` and `If-Range` were
present, a bounded conditional-result value, and short SHA-256 fingerprints for
the emitted and requested validators. The raw validators and media path are not
logged. The fingerprints are only correlation aids and are not protocol
validators.

Playback sessions already treat each transport request independently.
Sequential ranged requests refresh transport activity, and cleanup never
expires a session while one of those transport requests is active. A late
request within the paused-session grace remains valid under the same rules as
the initial request.

The capability does not apply to progressive remux delivery
`server_remux_progressive`, which is not byte-resumable. It applies only to
`original_http`; remux and transcode transports retain their existing
contracts.
