//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	"skill-atlas/internal/htmlreport"
	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
)

// screenshotsEnv is set by screenshots.sh, which runs the tests in the pinned Linux container the baselines come from.
const screenshotsEnv = "SKILL_ATLAS_E2E_SCREENSHOTS"

const (
	// A pixel differs when any channel is off by more than pixelTolerance (of 255).
	// Software rasterizing and font hinting are stable on one image, so this only absorbs anti-aliasing noise
	// between emulated (local) and native (CI) amd64 runs.
	pixelTolerance = 6
	// A shot fails when more than maxDiffPixels pixels differ. A changed link or word of text is 100+ pixels.
	maxDiffPixels = 25
)

var (
	desktop = playwright.Size{Width: 1280, Height: 800}
	phone   = playwright.Size{Width: 390, Height: 844}
)

// pageShot is one screenshot of a report page. It captures the viewport, not the full page.
type pageShot struct {
	name   string
	report string // key of the report to open
	size   playwright.Size
	dark   bool
	mobile bool
	anchor string                                // fragment to open, e.g. "skill-3"; empty for the top
	act    func(t *testing.T, p playwright.Page) // runs after load, before the shot
}

// screenshotReports renders each report page once, from the fixtures the other tests already downloaded.
type screenshotReports struct {
	single, multi, empty string            // file:// URLs
	zoomIDs              map[string]string // "invalid" and "claude" -> section id
}

func TestScreenshots(t *testing.T) {
	t.Parallel()
	if os.Getenv(screenshotsEnv) != "1" {
		t.Skip("screenshot baselines are rendered in a pinned Linux container; run e2e/screenshots.sh (add -update to rewrite them)")
	}

	rep := buildScreenshotReports(t)
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("start playwright (screenshots.sh installs the driver and Chromium): %v", err)
	}
	t.Cleanup(func() { pw.Stop() })
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Args: []string{"--font-render-hinting=none"},
	})
	if err != nil {
		t.Fatalf("launch chromium: %v", err)
	}
	t.Cleanup(func() { browser.Close() })

	urls := map[string]string{"single": rep.single, "multi": rep.multi, "empty": rep.empty}
	filter := func(query string) func(*testing.T, playwright.Page) {
		return func(t *testing.T, p playwright.Page) {
			if err := p.Keyboard().Press("/"); err != nil {
				t.Fatal(err)
			}
			if err := p.Keyboard().Type(query); err != nil {
				t.Fatal(err)
			}
			if err := p.Locator("#fcount:not([hidden])").WaitFor(); err != nil {
				t.Fatal(err)
			}
		}
	}
	sortZA := func(t *testing.T, p playwright.Page) {
		if _, err := p.Locator("#sort").SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("desc")}); err != nil {
			t.Fatal(err)
		}
		// Blur, so the shot doesn't depend on the focus ring.
		if err := p.Locator("#sort").Blur(); err != nil {
			t.Fatal(err)
		}
	}
	shots := []pageShot{
		{name: "zoom-light", report: "single", size: desktop},
		{name: "zoom-dark", report: "single", size: desktop, dark: true},
		{name: "zoom-phone", report: "single", size: phone, mobile: true},
		{name: "zoom-filter", report: "single", size: desktop, act: filter("laconic")},
		{name: "zoom-invalid-skill", report: "single", size: desktop, anchor: rep.zoomIDs["invalid"]},
		{name: "zoom-claude-fields", report: "single", size: desktop, anchor: rep.zoomIDs["claude"]},
		{name: "zoom-sort-za", report: "single", size: desktop, act: sortZA},
		{name: "multi-repo-failed", report: "multi", size: desktop},
		{name: "multi-repo-sort-za", report: "multi", size: desktop, act: sortZA},
		{name: "ideavim-no-skills", report: "empty", size: desktop},
	}
	for _, s := range shots {
		t.Run(s.name, func(t *testing.T) {
			u := urls[s.report]
			if s.anchor != "" {
				u += "#" + s.anchor
			}
			checkShot(t, s.name, capture(t, browser, s, u))
		})
	}
}

// capture opens the page in a fresh context with fixed viewport, scale, locale and motion, and returns the PNG.
func capture(t *testing.T, browser playwright.Browser, s pageShot, u string) []byte {
	t.Helper()
	scheme := playwright.ColorSchemeLight
	if s.dark {
		scheme = playwright.ColorSchemeDark
	}
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:          &s.size,
		DeviceScaleFactor: playwright.Float(1),
		IsMobile:          playwright.Bool(s.mobile),
		ColorScheme:       scheme,
		ReducedMotion:     playwright.ReducedMotionReduce,
		Locale:            playwright.String("en-US"),
		TimezoneId:        playwright.String("UTC"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(u, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	// The page loads no web fonts; this waits for the system fallback to settle.
	if _, err := page.Evaluate("document.fonts.ready.then(() => true)"); err != nil {
		t.Fatal(err)
	}
	if s.act != nil {
		s.act(t, page)
	}
	png, err := page.Screenshot(playwright.PageScreenshotOptions{
		Animations: playwright.ScreenshotAnimationsDisabled,
		Caret:      playwright.ScreenshotCaretHide,
		Scale:      playwright.ScreenshotScaleCss,
	})
	if err != nil {
		t.Fatal(err)
	}
	return png
}

// buildScreenshotReports renders the 3 report pages in-process. Each fixture is downloaded once, by downloadScan.
func buildScreenshotReports(t *testing.T) screenshotReports {
	t.Helper()
	zoom := downloadScan(t, claudeSkills.url, claudeSkills.tag, scan.Options{})
	vim := downloadScan(t, ideavim.url, ideavim.tag, scan.Options{})
	vimNone := downloadScan(t, ideavim.url, ideavim.tag, scan.Options{Exclude: []string{".claude/"}})
	if len(vimNone.skills) != 0 || vimNone.excluded == 0 {
		t.Fatalf("excluding .claude/ left %d skills and %d excluded, want 0 and some", len(vimNone.skills), vimNone.excluded)
	}

	// A real failed download gives the real error text.
	bad := failedRepo(t)
	toRepo := func(s scanned) htmlreport.Repo {
		return htmlreport.Repo{
			Name: s.target.Display, Ref: s.checkout.Ref, SHA: s.checkout.SHA,
			Skills: s.skills, Excluded: s.excluded,
		}
	}
	// The page numbers the sections in name order A–Z.
	rep := screenshotReports{zoomIDs: map[string]string{}}
	for i, s := range sortedSkills(zoom.skills) {
		switch s.Path {
		case "_template/SKILL.md":
			rep.zoomIDs["invalid"] = fmt.Sprintf("skill-%d", i+1)
		case "skills/laconic/SKILL.md":
			rep.zoomIDs["claude"] = fmt.Sprintf("skill-%d", i+1)
		}
	}
	for k, id := range rep.zoomIDs {
		if id == "" {
			t.Fatalf("no id for the %s skill", k)
		}
	}
	if len(rep.zoomIDs) != 2 {
		t.Fatalf("zoom skills lack the invalid or the Claude Code fields skill: %v", rep.zoomIDs)
	}

	rep.single = writeReport(t, htmlreport.Report{Repos: []htmlreport.Repo{toRepo(zoom)}})
	rep.multi = writeReport(t, htmlreport.Report{Repos: []htmlreport.Repo{toRepo(vim), bad, toRepo(zoom)}})
	rep.empty = writeReport(t, htmlreport.Report{Repos: []htmlreport.Repo{toRepo(vimNone)}})
	return rep
}

// failedRepo downloads a repository that doesn't exist and returns it as a failed report entry.
func failedRepo(t *testing.T) htmlreport.Repo {
	t.Helper()
	const badRepoURL = "https://github.com/JetBrains/no-such-repo.git"
	target, err := repo.ParseURL(badRepoURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err = repo.Download(ctx, target, "", mkdir(t, "download-"), nil)
	if err == nil {
		t.Fatalf("download of %s succeeded, want a failure", badRepoURL)
	}
	return htmlreport.Repo{Name: target.Display, Err: err.Error()}
}

// writeReport renders r to a file in the scratch root and returns its file:// URL.
func writeReport(t *testing.T, r htmlreport.Report) string {
	t.Helper()
	page, err := htmlreport.Render(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mkdir(t, "shot-"), "report.html")
	if err := os.WriteFile(path, page, 0o644); err != nil {
		t.Fatal(err)
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func shotPath(name string) string { return filepath.Join("testdata", "screenshots", name+".png") }

// checkShot compares got with the baseline PNG, or rewrites it under -update.
func checkShot(t *testing.T, name string, got []byte) {
	t.Helper()
	path := shotPath(name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read baseline (create it with: e2e/screenshots.sh -update): %v", err)
	}
	want, err := png.Decode(bytes.NewReader(wantBytes))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	actual, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	res := compareImages(want, actual)
	t.Logf("%s: %d of %d pixels differ", name, res.differing, res.total)
	if res.ok() {
		return
	}
	saved := saveFailure(t, name, wantBytes, got, want, actual)
	t.Errorf("screenshot %s differs from %s: %s%s\nIf the change is intended, rewrite the baselines with: e2e/screenshots.sh -update",
		name, path, res, saved)
}

// diffResult is the outcome of comparing 2 images.
type diffResult struct {
	sizeMismatch      bool
	wantSize, gotSize image.Point
	differing, total  int
}

func (r diffResult) ok() bool {
	return !r.sizeMismatch && r.differing <= maxDiffPixels
}

func (r diffResult) String() string {
	if r.sizeMismatch {
		return fmt.Sprintf("size is %dx%d, baseline is %dx%d", r.gotSize.X, r.gotSize.Y, r.wantSize.X, r.wantSize.Y)
	}
	return fmt.Sprintf("%d of %d pixels differ by more than %d/255 (allowed %d)",
		r.differing, r.total, pixelTolerance, maxDiffPixels)
}

func compareImages(want, got image.Image) diffResult {
	wb, gb := want.Bounds(), got.Bounds()
	r := diffResult{wantSize: wb.Size(), gotSize: gb.Size()}
	if wb.Size() != gb.Size() {
		r.sizeMismatch = true
		return r
	}
	r.total = wb.Dx() * wb.Dy()
	forDiffPixels(want, got, func(x, y int) { r.differing++ })
	return r
}

// forDiffPixels calls fn for each pixel (relative to the bounds origin) that differs beyond pixelTolerance.
func forDiffPixels(want, got image.Image, fn func(x, y int)) {
	wb, gb := want.Bounds(), got.Bounds()
	for y := range wb.Dy() {
		for x := range wb.Dx() {
			wr, wg, wbl, wa := want.At(wb.Min.X+x, wb.Min.Y+y).RGBA()
			gr, gg, gbl, ga := got.At(gb.Min.X+x, gb.Min.Y+y).RGBA()
			if channelDiff(wr, gr) || channelDiff(wg, gg) || channelDiff(wbl, gbl) || channelDiff(wa, ga) {
				fn(x, y)
			}
		}
	}
}

// channelDiff reports whether 2 16-bit channel values differ by more than pixelTolerance in 8-bit terms.
func channelDiff(a, b uint32) bool {
	d := int(a>>8) - int(b>>8)
	return d > pixelTolerance || d < -pixelTolerance
}

// saveFailure writes expected.png, actual.png and diff.png to $E2E_ARTIFACTS_DIR/screenshots/<name>/ and returns a note for the failure message.
func saveFailure(t *testing.T, name string, wantPNG, gotPNG []byte, want, got image.Image) string {
	dir := os.Getenv("E2E_ARTIFACTS_DIR")
	if dir == "" {
		return ""
	}
	dir = filepath.Join(dir, "screenshots", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("save screenshot artifacts: %v", err)
		return ""
	}
	files := map[string][]byte{"expected.png": wantPNG, "actual.png": gotPNG}
	if want.Bounds().Size() == got.Bounds().Size() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, diffImage(want, got)); err == nil {
			files["diff.png"] = buf.Bytes()
		}
	}
	for f, data := range files {
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Logf("save %s: %v", f, err)
		}
	}
	names := make([]string, 0, len(files))
	for f := range files {
		names = append(names, f)
	}
	slices.Sort(names)
	return fmt.Sprintf("\nSaved %v in %s", names, dir)
}

// diffImage returns got dimmed to a quarter of its brightness, with the differing pixels in red.
func diffImage(want, got image.Image) image.Image {
	b := got.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			r, g, bl, _ := got.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 10), G: uint8(g >> 10), B: uint8(bl >> 10), A: 255})
		}
	}
	forDiffPixels(want, got, func(x, y int) { out.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255}) })
	return out
}
