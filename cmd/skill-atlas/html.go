package main

import (
	"fmt"
	"io"

	"skill-atlas/internal/htmlreport"
)

// showHTML writes the report to a temp file and opens it with open. A failed open only warns.
func showHTML(results []scanned, open func(url string) error, stderr io.Writer) error {
	report := htmlreport.Report{Repos: make([]htmlreport.Repo, len(results))}
	for i, r := range results {
		report.Repos[i] = htmlreport.Repo{
			Name:     r.target.Display,
			Ref:      r.shownRef(),
			SHA:      r.checkout.SHA,
			Skills:   r.res.Skills,
			Excluded: r.res.Excluded,
		}
		if r.err != nil {
			report.Repos[i].Err = r.err.Error()
		}
	}
	page, err := htmlreport.Render(report)
	if err != nil {
		return err
	}
	path, err := htmlreport.WriteFile(page)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Report: %s\n", path)
	if err := open(htmlreport.FileURL(path)); err != nil {
		fmt.Fprintf(stderr, "skill-atlas: couldn't open a browser: %v\nOpen the report yourself: %s\n", err, path)
	}
	return nil
}
