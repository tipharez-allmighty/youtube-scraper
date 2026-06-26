# youtube-scraper

A CLI tool that scrapes YouTube videos and comments based on search queries and stores everything locally in a SQLite database for later analysis.

You give it a list of search queries, it fans out across YouTube's API — searching for videos, fetching comment threads, and pulling individual comments — all concurrently. Workers run in parallel pipelines connected by channels, so searching, thread fetching, and comment fetching all happen at the same time rather than waiting on each other. Every result gets persisted to SQLite as it comes in, so even if you stop mid-run you don't lose what was already collected.

> **Note:** `resume` command is not implemented yet. Interrupted jobs cannot be continued from where they left off.

---

## Setup

**Build:**

```bash
go build -o yt-scraper ./cmd/main.go
```

**Configure** via environment variables:

```bash
export YOUTUBE_API_KEY=your_api_key_here
export STATE_FILE=storage.db   # optional, default: storage.db
export NUM_WORKERS=5           # optional, default: 5
export BUFFER_SIZE=10          # optional, default: 10
```

Alternatively, place a `.env` file in the directory you run the binary from and it will be picked up automatically.

---

## Commands

### `run` — start a scrape job

Input is passed as JSON via stdin:

```bash
./yt-scraper run <<< '{
  "queries": [
    { "text": "golang concurrency" },
    { "text": "rust vs go" }
  ],
  "max_results_per_query": 10,
  "max_pages": 3,
  "max_threads": 2,
  "max_comments": 5
}'
```

Or from a file:

```bash
./yt-scraper run < input.json
```

**Input fields:**

| Field | Description |
|---|---|
| `queries` | List of search queries. Each has `text` and optional `order`, `published_after`, `published_before` |
| `max_results_per_query` | Results per page (1–50) |
| `max_pages` | How many pages of video results to fetch per query |
| `max_threads` | How many pages of comment threads to fetch per video |
| `max_comments` | How many pages of replies to fetch per thread |
| `state_file` | Override the SQLite path for this job (takes priority over `STATE_FILE` env var) |

---

### `jobs` — manage past jobs

List recent jobs (defaults to last 5):

```bash
./yt-scraper jobs
./yt-scraper jobs list
./yt-scraper jobs list -l 20
./yt-scraper jobs list -s /path/to/storage.db
```

Check task counts for a specific job:

```bash
./yt-scraper jobs status <job-id>
./yt-scraper jobs status <job-id> -s /path/to/storage.db
```

The `-s` flag on both `list` and `status` lets you point at a different SQLite file without changing your environment.

---

## Help

```bash
./yt-scraper --help
./yt-scraper run --help
./yt-scraper jobs --help
./yt-scraper jobs list --help
./yt-scraper jobs status --help
```
