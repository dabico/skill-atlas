package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"skill-atlas/internal/repo"
)

// checkOrgs applies the organization rules to srcs: an organization takes no ref and appears once.
// It drops the repositories whose owner is an organization in srcs, since the organization scan
// covers them. msg is a usage error, or "".
func checkOrgs(srcs []source) (kept []source, msg string) {
	orgs := map[string]bool{}
	for _, s := range srcs {
		if !s.target.Org {
			continue
		}
		if s.ref != "" {
			return nil, fmt.Sprintf("%s is an organization, #%s isn't supported", s.target.Display, s.ref)
		}
		owner := strings.ToLower(s.target.Owner)
		if orgs[owner] {
			return nil, fmt.Sprintf("%s given twice", s.target.Display)
		}
		orgs[owner] = true
	}
	for _, s := range srcs {
		if s.target.Org || !orgs[strings.ToLower(s.target.Owner)] {
			kept = append(kept, s)
		}
	}
	return kept, ""
}

// expandOrgs replaces each organization in srcs with its repositories. It lists the organizations
// one at a time, in order. An organization that can't be listed stays as 1 failed entry, printed
// to progress when srcs has several entries. The error is non-nil only when ctx is cancelled.
func expandOrgs(ctx context.Context, srcs []source, list listFunc, progress io.Writer) ([]scanned, error) {
	var out []scanned
	for _, s := range srcs {
		if !s.target.Org {
			out = append(out, scanned{source: s})
			continue
		}
		fmt.Fprintf(progress, "Listing repositories in %s…\n", s.target.Display)
		repos, err := list(ctx, s.target)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			e := scanned{source: s, err: err}
			if len(srcs) > 1 {
				printFailure(progress, e)
				e.printed = true
			}
			out = append(out, e)
			continue
		}
		for _, t := range repos {
			out = append(out, scanned{source: source{target: t, org: &s.target}})
		}
	}
	return out, nil
}

// dropEmpty leaves out the empty repositories that came from an organization. An organization
// whose repositories are all empty becomes 1 failed entry.
func dropEmpty(entries []scanned) []scanned {
	var out []scanned
	for i := 0; i < len(entries); {
		org := entries[i].org
		if org == nil {
			out = append(out, entries[i])
			i++
			continue
		}
		n := len(out)
		for ; i < len(entries) && entries[i].org == org; i++ {
			if !entries[i].skipped() {
				out = append(out, entries[i])
			}
		}
		if len(out) == n {
			out = append(out, scanned{source: source{target: *org}, err: repo.NoRepositories(*org)})
		}
	}
	return out
}
