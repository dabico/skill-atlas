package main

import (
	"fmt"
	"io"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
)

// showHTML writes the report to a temp file and opens it with open. A failed open only warns.
func showHTML(target repo.Target, checkout repo.Checkout, res scan.Result, open func(url string) error, stderr io.Writer) error {
	page, err := htmlreport.Render(htmlreport.Report{
		Repo:     target.Display,
		Ref:      checkout.Ref,
		SHA:      checkout.SHA,
		Skills:   res.Skills,
		Excluded: res.Excluded,
	})
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
