package storage

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"tipharez-allmighty/youtube-scraper/internal/input"
)

func setupTestDBStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open test connection: %v", err)
	}
	db.SetMaxOpenConns(1)
	schema := strings.Join(schemas, "\n")
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to load tables: %v", err)
	}
	store := NewStore(db)

	t.Cleanup(func() {
		db.Close()
	})
	return store
}

func TestStore(t *testing.T) {
	store := setupTestDBStore(t)
	input := input.InputSchema{
		Queries: []input.Query{
			{Text: "golang tutorial"},
		},
		MaxResultsPerQuery: 10,
	}

	job := Job{
		ID:    "job-1",
		Input: input,
	}
	task := Task{
		ID:      "task-0",
		JobID:   job.ID,
		Type:    Search,
		Status:  Running,
		Payload: "{}",
	}
	type taskTest struct {
		ID   string
		Type Type
	}
	testTasks := []taskTest{
		{ID: "task_videos", Type: Search},
		{ID: "task_threads", Type: Thread},
		{ID: "task_comments", Type: Reply},
	}
	videos := []Video{
		{
			ID:          "video-1",
			JobID:       "job-1",
			QueryText:   "golang tutorial",
			Title:       "Learn Go",
			Description: "A tutorial",
			PublishedAt: time.Now(),
		},
	}
	threads := []CommentThread{
		{
			CommentBase: CommentBase{
				ID:           "thread-1",
				JobID:        "job-1",
				Author:       "some author",
				TextDisplay:  "great video",
				TextOriginal: "great video",
				LikeCount:    5,
				PublishedAt:  time.Now(),
			},
			VideoID:         "video-1",
			TotalReplyCount: 1,
		},
	}

	comments := []Comment{
		{
			CommentBase: CommentBase{
				ID:           "comment-1",
				JobID:        "job-1",
				Author:       "some author",
				TextDisplay:  "totally agree",
				TextOriginal: "totally agree",
				LikeCount:    2,
				PublishedAt:  time.Now(),
			},
			ThreadID: "thread-1",
		},
	}
	if err := store.InsertJob(job); err != nil {
		t.Fatalf("failed to insert job")
	}
	t.Run("jobs and tasks operations", func(t *testing.T) {
		jobs, err := store.SelectJobs(1)
		if err != nil {
			t.Fatalf("failed to select jobs")
		}
		if len(jobs) != 1 {
			t.Fatalf("wrong amount of jobs %d != 1", len(jobs))
		}
		inputData, err := store.SelectJobInput(jobs[0].ID)
		if err != nil {
			t.Fatalf("failed to select jobs %v input", jobs[0].ID)
		}
		if input.Queries[0].Text != inputData.Queries[0].Text {
			t.Fatalf("wrong input saved in database")
		}

		if err := store.InsertTask(task); err != nil {
			t.Fatalf("failed to insert task")
		}
		runJobs, err := store.SelectJobs(1)
		if err != nil {
			t.Fatalf("failed to select jobs")
		}
		errMsg := errors.New("some error").Error()
		if err := store.FailRunningTasks(runJobs[0].ID, &errMsg); err != nil {
			t.Fatalf("failed to change status or running tasks to failed")
		}
		jobStatus, err := store.SelectJobsStatus(runJobs[0].ID)
		if err != nil {
			t.Fatalf("failed to select job status")
		}

		if jobStatus.Failed != 1 {
			t.Fatalf("wrong job status")
		}
		tasks, err := store.SelectFailedTasks(jobs[0].ID)
		if err != nil {
			t.Fatalf("failed to select failed tasks")
		}
		if len(tasks) != 1 {
			t.Fatalf("wrong amnount of failed tasks")
		}
		if *tasks[0].Error != errMsg {
			t.Fatalf("wrong error message %q != %q", *tasks[0].Error, errMsg)
		}
		emptyErr := ""
		if err != store.UpdateTaskStatus(tasks[0].ID, Done, &emptyErr) {
			t.Fatalf("failed to update status")
		}
		jobStatusDone, err := store.SelectJobsStatus(runJobs[0].ID)
		if err != nil {
			t.Fatalf("failed to select job status when checking Done")
		}

		if jobStatusDone.Failed != 0 || jobStatusDone.Running != 0 {
			t.Fatalf("wrong job status")
		}
	})
	t.Run("data insert and select", func(t *testing.T) {
		for _, testTask := range testTasks {
			newTask := Task{
				ID:      testTask.ID,
				JobID:   job.ID,
				Type:    testTask.Type,
				Status:  Running,
				Payload: "{}",
			}
			if err := store.InsertTask(newTask); err != nil {
				t.Fatalf("failed to insert task for videos")
			}
		}

		if err := store.CompleteTask(testTasks[0].ID, func(tx TxExecutable) error {
			return InsertVideos(tx, videos)
		}); err != nil {
			t.Fatalf("failed to complete video task")
		}
		if err := store.CompleteTask(testTasks[1].ID, func(tx TxExecutable) error {
			return InsertThreads(tx, threads)
		}); err != nil {
			t.Fatalf("failed to complete thread task")
		}
		if err := store.CompleteTask(testTasks[2].ID, func(tx TxExecutable) error {
			return InsertComments(tx, comments)
		}); err != nil {
			t.Fatalf("failed to complete comment task")
		}
		doneVideos, _, err := store.SelectVideos(job.ID, 1, 0)
		if err != nil {
			t.Fatalf("failed to select videos")
		}
		if doneVideos[0].ID != videos[0].ID {
			t.Fatalf("inserted videos IDs dont match done video id:%q != inserted video id:%q", doneVideos[0].ID, videos[0].ID)
		}
		doneThreads, _, err := store.SelectThreads(job.ID, 1, 0)
		if err != nil {
			t.Fatalf("failed to select threads")
		}
		if doneThreads[0].ID != threads[0].ID {
			t.Fatalf("inserted threads IDs dont match done thread id:%q != inserted thread id:%q", doneThreads[0].ID, threads[0].ID)
		}

		doneComments, _, err := store.SelectComments(job.ID, 1, 0)
		if err != nil {
			t.Fatalf("failed to select comments")
		}
		if doneComments[0].ID != comments[0].ID {
			t.Fatalf("inserted comments IDs dont match done comment id:%q != inserted comment id:%q", doneComments[0].ID, comments[0].ID)
		}
		jobStatusCompleted, err := store.SelectJobsStatus(job.ID)
		if err != nil {
			t.Fatalf("failed to select job status when checking Completed")
		}
		if jobStatusCompleted.Failed != 0 || jobStatusCompleted.Running != 0 {
			t.Fatalf("wrong job status")
		}
	})
}
