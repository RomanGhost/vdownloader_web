# vdownloader_web

Browser front end for [vdownloader_worker](../vdownloader_worker/README.md): a static HTML/CSS/JS page (embedded in the binary via `go:embed`) plus a thin Go bridge, since a browser can't speak AMQP directly.

## How it works

`main.go` has no business logic of its own beyond one thing: bridging `POST /api/jobs` to RabbitMQ.

| Route | Handling |
|---|---|
| `GET /`, static assets | Served from the embedded `static/` directory |
| `GET /api/formats?url=` | Reverse-proxied straight to the worker |
| `GET /api/jobs/{file_id}` | Reverse-proxied straight to the worker (client-side polling) |
| `GET /files/{file_id}` | Reverse-proxied straight to the worker |
| `POST /api/jobs` | **Not** proxied — handled locally: publishes the job request to the `video.jobs` RabbitMQ queue (the worker no longer accepts job submissions over HTTP), generates `file_id`, returns it as `{"file_id": "..."}` |

See [main.go](main.go) (`handleCreateJob`).

## User flow ([static/app.js](static/app.js))

1. User pastes a URL, page calls `GET /api/formats?url=`.
2. Renders step 1: one button per available quality tier (up to 2160p/4K, capped to the source's real max) plus **🎵 Audio only**.
3. **Quality tier clicked** → step 2: **🔊 With audio** / **🔇 Without audio**.
   **Audio only clicked** → step 2: **MP3 (default)** / **M4A** / **OPUS** / **WAV**.
4. On a step-2 pick, `POST /api/jobs` with `{url, title, kind: "video"|"audio", height, with_audio}` or `{..., audio_format}`.
5. Page polls `GET /api/jobs/{file_id}` every 2s until `status` is `ready` (shows a download link to `/files/{file_id}`) or `failed` (shows the error).

There is no server-push here — unlike the Telegram bot, this service does not consume the `video.completed` queue; the browser just polls.

## Configuration

Env vars, read via `.env` → environment:

| Env var | Default | Meaning |
|---|---|---|
| `WORKER_URL` | `http://localhost:8080` | Worker base URL — target for the reverse proxy |
| `HTTP_ADDR` | `:8082` | Address this service listens on |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection URL |

`POST /api/jobs` publishes to the `video.jobs` queue — a fixed constant (`internal/mq`), not configuration.

## Running

```bash
go build -o webui .
./webui
```

### Docker

```bash
docker build -t vdownloader-web .
docker run -it -p 8082:8082 \
  -e WORKER_URL=http://worker:8080 -e RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/ \
  vdownloader-web
```

See the repo root [docker-compose.yml](../docker-compose.yml) to run it alongside the worker and RabbitMQ.

## Job request wire format

Must stay in sync with the worker's contract, documented in [vdownloader_worker/README.md#rabbitmq-contract](../vdownloader_worker/README.md#rabbitmq-contract). `POST /api/jobs` body:

```json
{"url": "https://...", "title": "optional", "duration": 635, "kind": "video", "height": 1080, "with_audio": true}
{"url": "https://...", "kind": "audio", "audio_format": "opus"}
```
`kind` must be `"video"` or `"audio"`; `height` is required (and must be > 0) when `kind == "video"`. `file_id` is generated server-side and ignored if the client sends one. `duration` is optional — [static/app.js](static/app.js) captures it from the `GET /api/formats` response (`currentDuration`) and echoes it back so the worker can size its download timeout without a second lookup.

## Project structure

```
.
├── main.go                        # Reverse proxy + POST /api/jobs → RabbitMQ bridge
├── main_test.go
├── internal/
│   ├── config/
│   │   ├── config.go              # Env var loading
│   │   └── config_test.go
│   └── mq/
│       ├── mq.go                  # Queue names + durable-queue declare helper
│       ├── publisher.go           # Reconnecting persistent-message publisher
│       └── consumer.go            # Reconnecting queue consumer (unused here; shared package)
└── static/                        # Embedded via go:embed
    ├── index.html
    ├── style.css
    └── app.js                     # Two-step quality/audio picker, job polling
```

## Testing

```bash
go test ./...
```

No live worker or RabbitMQ broker needed: `handleCreateJob` takes a `jobPublisher` interface (satisfied by `*mq.Publisher` in `main()`, and by an in-memory fake in tests) specifically so its validation and response logic can be tested without a real broker.

- `internal/config/config_test.go` — env var defaults/overrides.
- `main_test.go` — `handleCreateJob`: valid video/audio requests publish the right JSON and return `{"file_id": ...}`; a client-supplied `file_id` is ignored in favor of a server-generated one; missing `url` / invalid `kind` / `kind == "video"` without `height` all return `400` without publishing anything; a publish failure returns `500`; non-`POST` methods fall through to the reverse proxy untouched.

Not covered: the reverse-proxy wiring itself (`GET /api/formats`, `GET /api/jobs/*`, `/files/*` → the worker) and `static/app.js`. Both are exercised by the [repo root's end-to-end smoke test](../README.md#testing), which drives the real `POST /api/jobs` → poll → download flow through this service against a live worker.
