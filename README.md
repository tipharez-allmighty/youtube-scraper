# youtube-scraper

A CLI tool that scrapes YouTube videos and comments based on search queries and stores everything locally in a SQLite database for later analysis.

You give it a list of search queries, it fans out across YouTube's API — searching for videos, fetching comment threads, and pulling individual comments — all concurrently. Workers run in parallel pipelines connected by channels, so searching, thread fetching, and comment fetching all happen at the same time rather than waiting on each other. Every result gets persisted to SQLite as it comes in, so even if you stop mid-run you don't lose what was already collected. If a job gets interrupted or a task fails, use `resume` to pick it back up from where it left off.

---

## Setup

**Build and install:**

```bash
go build -o yt-scraper ./cmd/main.go
mv yt-scraper /usr/local/bin/
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

**Intended usage:**

```bash
yt-scraper run -f yaml < example-input.yaml
```

YAML is the recommended way to define input — comments make each field
self-explanatory, and unlike a one-off JSON string on the command line, a
YAML file can be saved and kept around. Keep a folder of them (one per
search you care about) and it's immediately clear what each one does
without opening `run` --help or the source.

Input is JSON or YAML, always piped in via stdin. Format defaults to `json`;
pass `-f yaml` to send YAML instead.

```bash
yt-scraper run <<< '{
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

See [`example-input.json`](./example-input.json) and
[`example-input.yaml`](./example-input.yaml) for a full reference covering
every field the input accepts, with comments on what's required vs optional.

**Piping a file in, per shell:**

```bash
# bash / zsh (Linux, macOS, WSL)
yt-scraper run -f json < example-input.json
yt-scraper run -f yaml < example-input.yaml
```

```powershell
# PowerShell (Windows)
Get-Content example-input.yaml | yt-scraper run -f yaml
```

```cmd
:: cmd.exe (Windows)
yt-scraper run -f yaml < example-input.yaml
```

`<` works the same across bash, zsh, and cmd.exe — it hands the file's
contents to the program as stdin. PowerShell doesn't support `<` for
redirection, so use `Get-Content file | yt-scraper run -f ...` instead.

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
yt-scraper jobs
yt-scraper jobs list
yt-scraper jobs list -l 20
yt-scraper jobs list -s /path/to/storage.db
```

Check task counts for a specific job:

```bash
yt-scraper jobs status <job-id>
yt-scraper jobs status <job-id> -s /path/to/storage.db
```

The `-s` flag on both `list` and `status` lets you point at a different SQLite file without changing your environment.

---

### `resume` — retry failed tasks from a past job

```bash
yt-scraper resume <job-id>
yt-scraper resume <job-id> -s /path/to/storage.db
```

Looks up the failed tasks for `<job-id>` and re-runs them starting from the earliest failed stage (search → threads → comments), reusing the original job's settings (`max_pages`, `max_threads`, `max_comments`). API calls are retried with exponential backoff on failure, same as during `run`. If no failed tasks are found for the job, it exits with an error.

The `-s` flag works the same as on `jobs list`/`jobs status`.

```
