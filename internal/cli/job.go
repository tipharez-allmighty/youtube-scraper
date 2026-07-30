package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"tipharez-allmighty/youtube-scraper/internal/config"
)

type JobsCmd struct {
	List   JobsListCmd   `cmd:"list" help:"List jobs" default:"list"`
	Status JobsStatusCmd `cmd:"status" help:"Show job task counts"`
}

type JobsListCmd struct {
	Limit     int    `short:"l" default:"5" help:"Max jobs to show"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

type JobsStatusCmd struct {
	JobID     string `arg:"" help:"Job ID for checking job status"`
	StateFile string `short:"s" optional:"" help:"Path to state file"`
}

func (j *JobsListCmd) Run(cfg *config.Config) error {
	store, err := getStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	jobs, err := store.SelectJobs(j.Limit)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCREATED\tQUERIES")
	for _, job := range jobs {
		var queries []string
		for _, query := range job.Input.Queries {
			queries = append(queries, query.Text)
		}
		fmt.Fprintf(w, "%v\t%v\t%v\n", job.ID, job.CreatedAt.Format(time.DateTime), queries)
	}
	return w.Flush()
}

func (j *JobsStatusCmd) Run(cfg *config.Config) error {
	store, err := getStore(cfg, j.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load storage: %w", err)
	}
	defer store.Close()
	jobStatus, err := store.SelectJobsStatus(j.JobID)
	if err != nil {
		return err
	}
	var queries []string
	for _, query := range jobStatus.Input.Queries {
		queries = append(queries, query.Text)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tQUERIES\tTOTAL\tRUNNING\tFAILED\n")
	fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", jobStatus.ID, queries, jobStatus.Total, jobStatus.Running, jobStatus.Failed)
	return w.Flush()
}
