package main

import (
	"context"
	"io"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/skill"
)

// showHTML renders the scan and serves the page to the browser until it loads or ctx ends.
func showHTML(ctx context.Context, target repo.Target, checkout repo.Checkout, skills []skill.Skill, stderr io.Writer) error {
	page, err := htmlreport.Render(htmlreport.Report{
		Repo:   target.Display,
		Ref:    checkout.Ref,
		SHA:    checkout.SHA,
		Skills: skills,
	})
	if err != nil {
		return err
	}
	return htmlreport.Serve(ctx, page, htmlreport.OpenBrowser, stderr)
}
