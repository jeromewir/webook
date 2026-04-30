## Wework auto-book

Allows you to host a small webserver that listens to requests and makes a desk booking for a specific date at a specific WeWork location.

### Booking

Pass the booking date and WeWork name as query parameters:

```
curl -X POST 'http://localhost:8080/api/book?date=Feb%2018,%202025&wework=115%20Broadway'
```

To book multiple dates at the same WeWork location, use the batch endpoint:

```bash
curl -X POST 'http://localhost:8080/api/book/batch' \
  -H 'Content-Type: application/json' \
  -d '{"wework":"115 Broadway","dates":["Feb 18, 2025","Feb 19, 2025"]}'
```

Batch bookings run in parallel, up to 3 at a time.

Use the exact WeWork name when possible. Partial names are accepted only when they match a single location.
