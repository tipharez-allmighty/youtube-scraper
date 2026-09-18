// Package cli.
package cli

type CLI struct {
	Run     RunCmd    `cmd:"run" help:"Run a scraping job"`
	Resume  ResumeCmd `cmd:"resume" help:"Resume failed tasks"`
	Jobs    JobsCmd   `cmd:"jobs" help:"List jobs"`
	Export  ExportCmd `cmd:"export" help:"Export data in csv format"`
	Verbose int       `help:"Increase verbosity." short:"v" type:"counter"`
}
