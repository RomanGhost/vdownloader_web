# vdownloader_web

Browser front end for [vdownloader_worker](../vdownloader_worker/README.md): a static HTML/CSS/JS page (embedded in the binary via `go:embed`) plus a thin Go bridge, since a browser can't speak Kafka directly.

## How it works

`main.go` has no business logic of its own beyond one thing: bridging `POST /api/jobs` to Kafka.

| Route | Handling |
|---|---|
| `GET /`, static assets | Served from the embedded `static/` directory |
| `GET /api/formats?url=` | Reverse-proxied straight to the worker |
| `GET /api/jobs/{file_id}` | Reverse-proxied straight to the worker (client-side polling) |
| `GET /files/{file_id}` | Reverse-proxied straight to the worker |
| `POST /api/jobs` | **Not** proxied — handled locally: publishes the job request to the worker's job-requests Kafka topic (the worker no longer accepts job submissions over HTTP), generates `file_id`, returns it as `{"file_id": "..."}` |

See [main.go](main.go) (`handleCreateJob`).

## User flow ([static/app.js](static/app.js))

1. User pastes a URL, page calls `GET /api/formats?url=`.
2. Renders step 1: one button per available quality tier (up to 2160p/4K, capped to the source's real max) plus **🎵 Audio only**.
3. **Quality tier clicked** → step 2: **🔊 With audio** / **🔇 Without audio**.
   **Audio only clicked** → step 2: **MP3 (default)** / **M4A** / **OPUS** / **WAV**.
4. On a step-2 pick, `POST /api/jobs` with `{url, title, kind: "video"|"audio", height, with_audio}` or `{..., audio_format}`.
5. Page polls `GET /api/jobs/{file_id}` every 2s until `status` is `ready` (shows a download link to `/files/{file_id}`) or `failed` (shows the error).

There is no server-push here — unlike the Telegram bot, this service does not consume the `video.completed` Kafka topic; the browser just polls.

## Configuration

Env vars, read via `.env` → environment:

| Env var | Default | Meaning |
|---|---|---|
| `WORKER_URL` | `http://localhost:8080` | Worker base URL — target for the reverse proxy |
| `HTTP_ADDR` | `:8082` | Address this service listens on |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated broker list |
| `KAFKA_JOBS_TOPIC` | `video.jobs` | Topic `POST /api/jobs` publishes to |

## Running

```bash
go build -o webui .
./webui
```

### Docker

```bash
docker build -t vdownloader-web .
docker run -it -p 8082:8082 \
  -e WORKER_URL=http://worker:8080 -e KAFKA_BROKERS=kafka:9092 \
  vdownloader-web
```

See the repo root [docker-compose.yml](../docker-compose.yml) to run it alongside the worker and Kafka.

## Job request wire format

Must stay in sync with the worker's contract, documented in [vdownloader_worker/README.md#kafka-contract](../vdownloader_worker/README.md#kafka-contract). `POST /api/jobs` body:

```json
{"url": "https://...", "title": "optional", "kind": "video", "height": 1080, "with_audio": true}
{"url": "https://...", "kind": "audio", "audio_format": "opus"}
```
`kind` must be `"video"` or `"audio"`; `height` is required (and must be > 0) when `kind == "video"`. `file_id` is generated server-side and ignored if the client sends one.

## Project structure

```
.
├── main.go                        # Reverse proxy + POST /api/jobs → Kafka bridge
├── internal/
│   └── config/
│       └── config.go              # Env var loading
└── static/                        # Embedded via go:embed
    ├── index.html
    ├── style.css
    └── app.js                     # Two-step quality/audio picker, job polling
```
