package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"tipharez-allmighty/youtube-scraper/internal/input"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db}
}

func (s *Store) SelectJobs(limit int) ([]Job, error) {
	rows, err := s.db.Query(`
	SELECT * FROM jobs
	ORDER by created_at DESC
	LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var input string
		if err := rows.Scan(
			&j.ID, &input, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(input), &j.Input); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *Store) SelectJobsStatus(jobID string) (*JobStatus, error) {
	row := s.db.QueryRow(`
	SELECT jobs.id, input, 
		COUNT(tasks.id),
		COUNT(CASE WHEN tasks.status = 'running' THEN 1 END),
		COUNT(CASE WHEN tasks.status = 'failed' THEN 1 END)
	FROM jobs
	LEFT JOIN tasks ON tasks.job_id = jobs.id
	WHERE jobs.id = ?
	GROUP BY jobs.id
	`, jobID)

	var j JobStatus
	var input string
	if err := row.Scan(
		&j.ID, &input, &j.Total, &j.Running, &j.Failed,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(input), &j.Input); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) SelectJobInput(jobID string) (*input.InputSchema, error) {
	row := s.db.QueryRow(`
	SELECT input 
	FROM jobs
	WHERE jobs.id = ?
	`, jobID)

	var jobInput string
	if err := row.Scan(
		&jobInput,
	); err != nil {
		return nil, err
	}
	var inputSchema input.InputSchema
	if err := json.Unmarshal([]byte(jobInput), &inputSchema); err != nil {
		return nil, err
	}
	return &inputSchema, nil
}

func (s *Store) SelectFailedTasks(jobID string) ([]Task, error) {
	rows, err := s.db.Query(`
	SELECT * FROM tasks
	WHERE tasks.job_id = ? AND tasks.status = 'failed'
	ORDER by created_at ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.JobID, &t.Type, &t.Status, &t.Payload, &t.PageToken, &t.Error, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Store) InsertJob(j Job) error {
	input, err := json.Marshal(j.Input)
	if err != nil {
		return fmt.Errorf("failed to marshal insert job input: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO jobs (id, input)
		VALUES (?,?)
		`, j.ID, string(input),
	)
	return err
}

func (s *Store) InsertTask(t Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, job_id, type, status, payload, page_token, error)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		status = excluded.status,
		error = excluded.error
		`, t.ID, t.JobID, t.Type, t.Status, t.Payload, t.PageToken, t.Error)
	return err
}

func (s *Store) UpdateTaskStatus(id string, status Status, error *string) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET status = ?, error = ? WHERE id = ?`, status, error, id,
	)
	return err
}

func (s *Store) FailRunningTasks(jobID string, error *string) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET status = 'failed', error = ? WHERE job_id = ? AND status = 'running'`, error, jobID,
	)
	return err
}

func (s *Store) CompleteTask(taskID string, insertFn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertFn(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE tasks SET status = 'done' WHERE id = ?`, taskID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func InsertVideos(tx *sql.Tx, videos []Video) error {
	for _, v := range videos {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO videos (id, job_id, query_text, title, description)
			VALUES (?,?,?,?,?)`, v.ID, v.JobID, v.QueryText, v.Title, v.Description); err != nil {
			return err
		}
	}
	return nil
}

func InsertThreads(tx *sql.Tx, threads []CommentThread) error {
	for _, t := range threads {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO threads (id, video_id, job_id, author, text_display, text_original, like_count, total_reply_count, published_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.VideoID, t.JobID, t.Author, t.TextDisplay, t.TextOriginal, t.LikeCount, t.TotalReplyCount, t.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func InsertComments(tx *sql.Tx, threads []Comment) error {
	for _, t := range threads {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO comments (id, thread_id, job_id, author, text_display, text_original, like_count, published_at) VALUES (?,?,?,?,?,?,?,?)`, t.ID, t.ThreadID, t.JobID, t.Author, t.TextDisplay, t.TextOriginal, t.LikeCount, t.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}
