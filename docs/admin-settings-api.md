# Administrator settings inspection

The following native v2 operations require an acting administrator and return
no-store responses:

- `GET /api/v2/admin/settings`: stored settings, excluding secret values and
  machine-managed keys.
- `GET /api/v2/admin/settings/effective`: active settings with the runtime's
  defaults applied, using the same redaction policy as stored settings.
- `GET /api/v2/admin/settings/restart-keys`: the compiled `keys` and `prefixes`
  whose changes need a server restart.
- `GET /api/v2/admin/settings/sensitive-status`: configured secret key names and
  `managed_by_env` names. Values are never returned. Both fields are arrays,
  including when empty.

Stored and effective settings are string dictionaries keyed by the server
settings registry. An empty dictionary is `{}`. Inspection copies stored values
before redaction so a cached settings map remains intact. Missing settings storage
returns `503 dependency_unavailable`; other storage failures return a generic
problem without database error details. Restart metadata is available without
settings storage.

The web settings pages use the v2 effective settings, restart metadata, and
sensitive-status operations. Writes remain on their existing bridge operations
until the corresponding mutation contracts migrate. The migration inventory lists
no Apple or Android consumers for these inspection operations. Jellyfin
compatibility does not expose these administrator settings contracts.

`POST /api/v2/admin/settings/check/{kind}` performs one synchronous connection
check against the submitted `values` and `dirty_keys`, merged with stored settings.
Supported kinds are `s3_public`, `s3_operational`, `s3_private`, `redis`,
`recommendations_embedding`, `ai_chat`, `ai_transcription`, `meilisearch`, and
`mdblist`. Existing endpoint-change protection for stored AI credentials applies.
Provider failures return `success: false` with a generic message that excludes
provider error bodies and credentials. Invalid kinds/configuration return `422`.

Checks can write temporary storage objects or incur provider charges. They return
a synchronous result, not a persisted job. The web sends each user-triggered check
once and disables mutation retries; a lost response must not trigger automatic
replay. This corrects the inventory's earlier assumption that every check was
read-only. Demo mode blocks this operation.
