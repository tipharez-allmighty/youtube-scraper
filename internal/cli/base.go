// Package cli.
package cli

type CLI struct {
	Run    RunCmd    `cmd:"run" help:"Run a scraping job"`
	Jobs   JobsCmd   `cmd:"jobs" help:"List jobs"`
	Resume ResumeCmd `cmd:"resume" help:"Resume failed tasks"`
}
