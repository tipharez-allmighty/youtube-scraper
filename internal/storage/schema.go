package storage

const jobSchema = `
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    input TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);`

const taskSchema = `
CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    job_id      TEXT NOT NULL REFERENCES jobs(id),
    type        TEXT NOT NULL CHECK (type IN ('search', 'thread', 'reply')),
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    payload     TEXT NOT NULL,
    page_token  TEXT,
		error       TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);`

const videoSchema = `
CREATE TABLE IF NOT EXISTS videos (
    id          TEXT NOT NULL,
    job_id      TEXT NOT NULL REFERENCES jobs(id),
    query_text  TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    published_at DATETIME NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),

		PRIMARY KEY (id, job_id)
);`

const threadSchema = `
CREATE TABLE IF NOT EXISTS threads (
    id                TEXT NOT NULL, 
    video_id          TEXT NOT NULL, 
    job_id            TEXT NOT NULL REFERENCES jobs(id),
    author            TEXT NOT NULL,
    text_display      TEXT NOT NULL,
    text_original     TEXT NOT NULL,
    like_count        INTEGER NOT NULL DEFAULT 0,
    total_reply_count INTEGER NOT NULL DEFAULT 0,
    published_at      DATETIME NOT NULL,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    
    PRIMARY KEY (id, job_id),
    FOREIGN KEY (video_id, job_id) REFERENCES videos(id, job_id)
);`

const commentSchema = `
CREATE TABLE IF NOT EXISTS comments (
		id            TEXT NOT NULL, 
    thread_id     TEXT NOT NULL, 
    job_id        TEXT NOT NULL REFERENCES jobs(id),
    author        TEXT NOT NULL,
    text_display  TEXT NOT NULL,
    text_original TEXT NOT NULL,
    like_count    INTEGER NOT NULL DEFAULT 0,
    published_at  DATETIME NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    
    PRIMARY KEY (id, job_id),
    FOREIGN KEY (thread_id, job_id) REFERENCES threads(id, job_id)
);`

const indexes = `
CREATE INDEX IF NOT EXISTS idx_tasks_job_id ON tasks(job_id);
`
