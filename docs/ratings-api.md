# Ratings API

Silo ratings have two deliberately separate uses:

- A personal rating belongs to one account profile and remains an input to that profile's recommendation taste.
- A community rating surface shows how watched profiles on the same Silo server rated an item. Its average and reactions are display-only and never affect recommendations.

All documented data and mutation routes require an authenticated, selected profile. Existing personal-rating routes and behavior are unchanged.

## Capability discovery

`GET /api/v1/ratings/capabilities`

```json
{
  "community_ratings": true,
  "community_rating_reactions": true
}
```

Clients should check this endpoint before presenting the optional community surface. The web client supports it on movie and main series detail pages. Apple, Android, and Jellyfin-compatible clients do not currently expose this Silo-specific surface.

## List community ratings

`GET /api/v1/ratings/{item_id}/community?limit=100`

Only ratings from profiles that watched the requested item are included. A movie is watched when its progress is complete or at least 50 percent. For a series, qualifying episode progress rolls up to the main series. Hidden history is excluded. Results are capped at 100 rows per request.

```json
{
  "average_rating": 4.5,
  "vote_count": 2,
  "ratings": [
    {
      "key": "opaque-rating-key",
      "display_name": "Sam***",
      "avatar_url": "/profile-avatars/avatar-1.svg",
      "rating": 5,
      "up_count": 3,
      "down_count": 1,
      "viewer_reaction": "up",
      "is_viewer": false
    }
  ]
}
```

`average_rating` is `null` when there are no qualifying votes. Profile and account IDs are never returned. `display_name` contains at most the first three Unicode characters followed by `***`. Avatar URLs are resolved from the profile's current avatar when the response is created, so later profile changes are reflected on refresh. Uploaded avatars use a short-lived opaque proxy URL; private storage keys are not sent to other profiles.

## React to a rating

Set or change the selected profile's one reaction to another profile's rating:

`PUT /api/v1/ratings/{item_id}/community/{rating_key}/reaction`

```json
{ "reaction": "up" }
```

`reaction` is `up` or `down`. Remove the selected profile's reaction with:

`DELETE /api/v1/ratings/{item_id}/community/{rating_key}/reaction`

Both successful mutations return `204 No Content`. A profile cannot react to its own rating. Ratings and reactions are stored in PostgreSQL and survive normal server and container restarts.
