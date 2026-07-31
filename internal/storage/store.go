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

func (s *Store) Close() {
	s.db.Close()
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

func (s *Store) SelectVideos(jobID string, limit int, offset int) ([]Video, int, error) {
	rows, err := s.db.Query(`
			SELECT * FROM videos WHERE job_id = ? ORDER BY published_at ASC 
		LIMIT ?
		OFFSET ?`, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(
			&v.ID, &v.JobID, &v.QueryText, &v.Title, &v.Description, &v.PublishedAt, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return videos, len(videos) + offset, nil
}

func (s *Store) SelectThreads(jobID string, limit int, offset int) ([]CommentThread, int, error) {
	rows, err := s.db.Query(`
		SELECT * FROM threads WHERE job_id = ? ORDER BY published_at ASC
		LIMIT ?
		OFFSET ?`, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var threads []CommentThread
	for rows.Next() {
		var t CommentThread
		if err := rows.Scan(
			&t.ID, &t.JobID, &t.VideoID, &t.Author, &t.TextDisplay, &t.TextOriginal, &t.TotalReplyCount, &t.LikeCount, &t.PublishedAt, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		threads = append(threads, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return threads, len(threads) + offset, nil
}

func (s *Store) SelectComments(jobID string, limit int, offset int) ([]Comment, int, error) {
	rows, err := s.db.Query(`
		SELECT * FROM comments WHERE job_id = ? ORDER BY published_at ASC
		LIMIT ?
		OFFSET ?`, jobID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.JobID, &c.ThreadID, &c.Author, &c.TextDisplay, &c.TextOriginal, &c.LikeCount, &c.PublishedAt, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return comments, len(comments) + offset, nil
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

type TxExecutable interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (s *Store) CompleteTask(taskID string, insertFn func(TxExecutable) error) error {
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

func InsertVideos(tx TxExecutable, videos []Video) error {
	for _, v := range videos {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO videos (id, job_id, query_text, title, description, published_at)
			VALUES (?,?,?,?,?,?)`, v.ID, v.JobID, v.QueryText, v.Title, v.Description, v.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func InsertThreads(tx TxExecutable, threads []CommentThread) error {
	for _, t := range threads {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO threads (id, video_id, job_id, author, text_display, text_original, like_count, total_reply_count, published_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.VideoID, t.JobID, t.Author, t.TextDisplay, t.TextOriginal, t.LikeCount, t.TotalReplyCount, t.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func InsertComments(tx TxExecutable, threads []Comment) error {
	for _, t := range threads {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO comments (id, thread_id, job_id, author, text_display, text_original, like_count, published_at) VALUES (?,?,?,?,?,?,?,?)`, t.ID, t.ThreadID, t.JobID, t.Author, t.TextDisplay, t.TextOriginal, t.LikeCount, t.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CleanDataByJobID(jobID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	delStmts := []string{
		`DELETE FROM comments WHERE job_id != ?;`,
		`DELETE FROM threads WHERE job_id != ?;`,
		`DELETE FROM videos WHERE job_id != ?;`,
	}
	for _, stmt := range delStmts {
		if _, err := tx.Exec(stmt, jobID); err != nil {
			return err
		}
	}
	dropStmts := []string{
		`DROP TABLE IF EXISTS tasks;`,
		`DROP TABLE IF EXISTS jobs;`,
	}
	for _, stmt := range dropStmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
