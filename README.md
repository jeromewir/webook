## Wework auto-book

Allows you to host a small webserver that listens to requests and makes a desk booking for a specific date at a specific WeWork location.

### API keys

All endpoints require an API key. Generate one before starting the server:

```bash
go run . apikey create --name my-client
```

The command prints the secret once. Save it somewhere safe and set `WEBOOK_API_KEY` in your client environment for the examples below. The CLI does not require WeWork credentials.

Keys are generated from 32 cryptographically random bytes. Only their SHA-256 hashes, names, and creation timestamps are stored in `api-keys.json`, created with owner-only permissions (`0600`). Set `API_KEYS_FILE` to use another path; its parent directory must already exist. Keep this file out of version control and container images when using a custom path.

Run the command again with a different name to add a key. The server caches hashes and rereads the file whenever a supplied key is unrecognized, so new keys work without a restart. If a reload fails, the request is rejected and the existing cache is retained. Server startup requires a valid, nonempty key file.

To revoke a key, remove its entry from the JSON array and restart the server. A successful reload also replaces the cache, but a removed key may still work until that happens.

For Docker Compose, create the key in the mounted data directory before starting the service:

```bash
mkdir -p data
API_KEYS_FILE=./data/api-keys.json go run . apikey create --name my-client
docker compose up -d
```

Compose mounts this directory at `/data` and sets `API_KEYS_FILE=/data/api-keys.json`. To add keys later using the running container:

```bash
docker compose exec webook /home/app apikey create --name another-client
```

### Booking

Pass the booking date and WeWork name as query parameters:

```
curl -X POST 'http://localhost:8080/api/book?date=Feb%2018,%202025&wework=115%20Broadway' \
  -H "Authorization: Bearer $WEBOOK_API_KEY"
```

To book multiple dates at the same WeWork location, use the batch endpoint:

```bash
curl -X POST 'http://localhost:8080/api/book/batch' \
  -H "Authorization: Bearer $WEBOOK_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"wework":"115 Broadway","dates":["Feb 18, 2025","Feb 19, 2025"]}'
```

Batch bookings run in parallel, up to 3 at a time.

Use the exact WeWork name when possible. Partial names are accepted only when they match a single location.

### Next bookings

List your upcoming bookings:

```bash
curl 'http://localhost:8080/api/bookings/next' \
  -H "Authorization: Bearer $WEBOOK_API_KEY"
```

Optionally pass `startDate` and `endDate` as `YYYY-MM-DD`:

```bash
curl 'http://localhost:8080/api/bookings/next?startDate=2026-03-01&endDate=2026-03-31' \
  -H "Authorization: Bearer $WEBOOK_API_KEY"
```
