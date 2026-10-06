package report

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xdearboy/prom-loadgen/results"
)

type engineStyle struct {
	title string
	color string
}

var engineOrder = []string{"prompp-0815", "prom-3150", "prom-2551"}

var engineStyles = map[string]engineStyle{
	"prompp-0815": {"Prom++ 0.8.15", "#2563eb"},
	"prom-3150":   {"Prometheus 3.15.0", "#ea580c"},
	"prom-2551":   {"Prometheus 2.55.1", "#059669"},
}

func styleOf(engine string) engineStyle {
	if s, ok := engineStyles[engine]; ok {
		return s
	}
	return engineStyle{engine, "#64748b"}
}

func (b *Bundle) chartEngines() []string {
	have := map[string]bool{}
	for _, e := range b.Engines() {
		have[e] = true
	}
	var out []string
	for _, e := range engineOrder {
		if have[e] {
			out = append(out, e)
			delete(have, e)
		}
	}
	rest := make([]string, 0, len(have))
	for e := range have {
		rest = append(rest, e)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

type bar struct {
	engine string
	value  float64
	text   string
}

type chart struct {
	title, subtitle, unit string
	labels                []string
	groups                [][]bar
	ref                   float64
}

func (b *Bundle) WriteCharts(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var charts = map[string]*chart{}
	if len(b.Ingests) > 0 {
		charts["rss-by-series.svg"] = b.byLevel("Resident memory", "engine process_resident_memory_bytes, average over the step, lower is better", "bytes",
			func(r *results.StepResources) float64 { return r.RSSSelfAvgBytes })
		charts["cpu-by-series.svg"] = b.byLevel("CPU while ingesting", "cgroup cpu cores, average over the step, lower is better", "cores",
			func(r *results.StepResources) float64 { return r.CPUCoresAvg })
		charts["disk-by-series.svg"] = b.byLevel("Data directory size", "end of the step, lower is better", "bytes",
			func(r *results.StepResources) float64 { return float64(r.DataDirBytes) })
	}
	for _, c := range b.concurrencyLevels() {
		charts[queryChart(c)] = b.queryLatency(c)
	}
	horizontal := map[string]*chart{"summary.svg": b.summary()}
	for name, c := range charts {
		horizontal[name] = c
	}
	files := map[string]string{
		"rss-timeline.svg":    b.rssTimeline(),
		"latency-scaling.svg": b.latencyScaling(),
	}
	for name, c := range horizontal {
		if c == nil {
			continue
		}
		if strings.HasPrefix(name, "query-") || name == "summary.svg" {
			files[name] = c.hsvg()
		} else {
			files[name] = c.svg()
		}
	}
	for name, body := range files {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bundle) byLevel(title, subtitle, unit string, get func(*results.StepResources) float64) *chart {
	c := &chart{title: title, subtitle: subtitle, unit: unit}
	for _, level := range b.seriesLevels() {
		var group []bar
		for _, e := range b.chartEngines() {
			if step := b.step(e, level); step != nil && step.Resources != nil {
				group = append(group, bar{engine: e, value: get(step.Resources)})
			}
		}
		c.groups = append(c.groups, group)
		c.labels = append(c.labels, fmt.Sprintf("%dk series", level/1000))
	}
	return c
}

func queryChart(concurrency int) string {
	return fmt.Sprintf("query-p50-c%d.svg", concurrency)
}

const chartQueries = 12

func (b *Bundle) queryLatency(concurrency int) *chart {
	names := b.slowestQueries(chartQueries)
	c := &chart{
		title:    fmt.Sprintf("Median query latency, concurrency %d", concurrency),
		subtitle: fmt.Sprintf("the %d slowest queries, lower is better", len(names)),
		unit:     "ms",
	}
	for _, name := range names {
		var group []bar
		for _, e := range b.chartEngines() {
			if m, ok := b.queryAt(e, name, concurrency); ok && m.Latency.Count > 0 {
				group = append(group, bar{engine: e, value: m.Latency.P50MS})
			}
		}
		c.groups = append(c.groups, group)
		c.labels = append(c.labels, name)
	}
	return c
}

func (b *Bundle) queryAt(engine, name string, concurrency int) (results.QueryMetric, bool) {
	for _, res := range b.QueriesFor(engine) {
		if res.Concurrency == concurrency {
			if m, ok := res.ByName(name); ok {
				return m, true
			}
		}
	}
	return results.QueryMetric{}, false
}

func (b *Bundle) slowestQueries(limit int) []string {
	best := map[string]float64{}
	for _, r := range b.Queries {
		for _, q := range r.Queries {
			best[q.Name] = math.Max(best[q.Name], q.Latency.P50MS)
		}
	}
	names := make([]string, 0, len(best))
	for n := range best {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if best[names[i]] != best[names[j]] {
			return best[names[i]] > best[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > limit {
		names = names[:limit]
	}
	return names
}

func niceStep(span float64, ticks int) float64 {
	raw := span / float64(ticks)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}

func axis(unit string, maxV float64) (step, top float64) {
	scale := 1.0
	if unit == "bytes" {
		scale = 1 << 20
		if maxV >= 2<<30 {
			scale = 1 << 30
		}
	}
	step = niceStep(maxV*1.08/scale, 5) * scale
	return step, math.Ceil(maxV*1.08/step) * step
}

func trim(v float64, unit string) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", v), "0"), ".") + unit
}

func fmtAxis(unit string, v float64) string {
	switch unit {
	case "bytes":
		switch {
		case v == 0:
			return "0"
		case v >= 1<<30:
			return trim(v/(1<<30), " GiB")
		}
		return trim(v/(1<<20), " MiB")
	case "ms":
		switch {
		case v == 0:
			return "0"
		case v >= 1000:
			return trim(v/1000, " s")
		}
		return trim(v, " ms")
	case "cores":
		return trim(v, "")
	case "pct":
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.0f", v)
}

func fmtShort(unit string, v float64) string {
	switch unit {
	case "bytes":
		if v >= 1<<30 {
			return fmt.Sprintf("%.1f GiB", v/(1<<30))
		}
		return fmt.Sprintf("%.0f MiB", v/(1<<20))
	case "ms":
		if v >= 1000 {
			return fmt.Sprintf("%.1f s", v/1000)
		}
		return fmt.Sprintf("%.0f ms", v)
	case "cores":
		return fmt.Sprintf("%.2f", v)
	case "pct":
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.0f", v)
}

const (
	svgFont  = `font-family="ui-sans-serif,system-ui,-apple-system,sans-serif"`
	svgInk   = "#0f172a"
	svgMuted = "#64748b"
	svgGrid  = "#e2e8f0"
	svgBand  = "#f8fafc"
)

func writeHeader(sb *strings.Builder, width, height float64, title, subtitle string, engines []string) {
	fmt.Fprintf(sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" %s>`+"\n", width, height, width, height, svgFont)
	sb.WriteString(`<rect width="100%" height="100%" fill="#ffffff" rx="10"/>` + "\n")
	fmt.Fprintf(sb, `<text x="28" y="34" font-size="17" font-weight="700" fill="%s">%s</text>`+"\n", svgInk, escape(title))
	fmt.Fprintf(sb, `<text x="28" y="54" font-size="12" fill="%s">%s</text>`+"\n", svgMuted, escape(subtitle))
	lx := width - 24
	for i := len(engines) - 1; i >= 0; i-- {
		st := styleOf(engines[i])
		lx -= float64(len(st.title))*6.6 + 26
		fmt.Fprintf(sb, `<rect x="%.1f" y="24" width="11" height="11" rx="3" fill="%s"/><text x="%.1f" y="34" font-size="12" font-weight="600" fill="#334155">%s</text>`+"\n", lx, st.color, lx+16, escape(st.title))
	}
}

func (c *chart) engines() []string {
	have := map[string]bool{}
	for _, g := range c.groups {
		for _, br := range g {
			have[br.engine] = true
		}
	}
	var out []string
	for _, e := range engineOrder {
		if have[e] {
			out = append(out, e)
		}
	}
	return out
}

func (c *chart) extent() (maxV float64, maxBars, maxLabel int) {
	for i, g := range c.groups {
		maxBars = max(maxBars, len(g))
		maxLabel = max(maxLabel, len(c.labels[i]))
		for _, br := range g {
			maxV = math.Max(maxV, br.value)
		}
	}
	return
}

func (c *chart) svg() string {
	const width, padL, padR, padT, plotH, padB = 980.0, 78.0, 24.0, 84.0, 320.0, 44.0
	maxV, maxBars, _ := c.extent()
	if maxBars == 0 || maxV == 0 {
		return ""
	}
	step, top := axis(c.unit, maxV)
	height := padT + plotH + padB
	plotW := width - padL - padR
	groupW := plotW / float64(len(c.groups))
	barW := math.Min(46, (groupW*0.78)/float64(maxBars))

	var sb strings.Builder
	writeHeader(&sb, width, height, c.title, c.subtitle, c.engines())
	for gi := range c.groups {
		if gi%2 == 1 {
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.0f" width="%.1f" height="%.0f" fill="%s"/>`+"\n", padL+float64(gi)*groupW, padT, groupW, plotH, svgBand)
		}
	}
	for v := 0.0; v <= top+step/2; v += step {
		y := padT + plotH - plotH*v/top
		fmt.Fprintf(&sb, `<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"/>`+"\n", padL, y, padL+plotW, y, svgGrid)
		fmt.Fprintf(&sb, `<text x="%.0f" y="%.1f" font-size="11" fill="%s" text-anchor="end">%s</text>`+"\n", padL-10, y+4, svgMuted, fmtAxis(c.unit, v))
	}
	for gi, g := range c.groups {
		cx := padL + (float64(gi)+0.5)*groupW
		x0 := cx - barW*float64(len(g))/2
		for bi, br := range g {
			h := plotH * br.value / top
			x := x0 + float64(bi)*barW
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="3" fill="%s"/>`+"\n", x+1.5, padT+plotH-h, barW-3, h, styleOf(br.engine).color)
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="10" font-weight="600" fill="#334155" text-anchor="middle">%s</text>`+"\n", x+barW/2, padT+plotH-h-5, fmtShort(c.unit, br.value))
		}
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="13" font-weight="600" fill="#1e293b" text-anchor="middle">%s</text>`+"\n", cx, padT+plotH+26, escape(c.labels[gi]))
	}
	sb.WriteString("</svg>\n")
	return sb.String()
}

func (c *chart) hsvg() string {
	const width, padT, padR, padB, barH, gap = 980.0, 84.0, 150.0, 40.0, 14.0, 18.0
	maxV, maxBars, maxLabel := c.extent()
	if maxBars == 0 || maxV == 0 {
		return ""
	}
	step, top := axis(c.unit, maxV)
	padL := math.Max(130, float64(maxLabel)*7+28)
	plotW := width - padL - padR
	rowH := float64(maxBars)*(barH+3) + gap
	plotH := rowH * float64(len(c.groups))
	height := padT + plotH + padB
	xOf := func(v float64) float64 { return padL + plotW*v/top }

	var sb strings.Builder
	writeHeader(&sb, width, height, c.title, c.subtitle, c.engines())
	for gi := range c.groups {
		if gi%2 == 0 {
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n", padL, padT+float64(gi)*rowH, plotW, rowH, svgBand)
		}
	}
	for v := 0.0; v <= top+step/2; v += step {
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.1f" stroke="%s"/><text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">%s</text>`+"\n", xOf(v), padT, xOf(v), padT+plotH, svgGrid, xOf(v), padT+plotH+20, svgMuted, fmtAxis(c.unit, v))
	}
	for gi, g := range c.groups {
		y0 := padT + float64(gi)*rowH + gap/2
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="12.5" font-weight="600" fill="#1e293b" text-anchor="end">%s</text>`+"\n", padL-12, padT+float64(gi)*rowH+rowH/2+4, escape(c.labels[gi]))
		for bi, br := range g {
			y := y0 + float64(bi)*(barH+3)
			w := plotW * br.value / top
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.0f" rx="3" fill="%s"/>`+"\n", padL, y, math.Max(w, 1), barH, styleOf(br.engine).color)
			text := br.text
			if text == "" {
				text = fmtShort(c.unit, br.value)
			}
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="11" font-weight="600" fill="#334155" stroke="#ffffff" stroke-width="3" paint-order="stroke">%s</text>`+"\n", padL+w+6, y+barH-3, escape(text))
		}
	}
	if c.ref > 0 {
		fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.1f" stroke="#475569" stroke-width="1.4" stroke-dasharray="5 4"/>`+"\n", xOf(c.ref), padT, xOf(c.ref), padT+plotH)
	}
	sb.WriteString("</svg>\n")
	return sb.String()
}

func (b *Bundle) rssTimeline() string {
	const (
		width, height, padL, padR, padT, padB = 980.0, 440.0, 78.0, 96.0, 84.0, 52.0
	)
	type line struct {
		engine string
		min    []float64
		val    []float64
	}
	var lines []line
	maxV, maxM := 0.0, 0.0
	for _, e := range b.chartEngines() {
		samples := b.Samples[e+"/ingest"]
		var l line
		l.engine = e
		var t0 time.Time
		for _, s := range samples {
			v, ok := s.Self["process_resident_memory_bytes"]
			if !ok {
				continue
			}
			if t0.IsZero() {
				t0 = s.TS
			}
			m := s.TS.Sub(t0).Minutes()
			l.min, l.val = append(l.min, m), append(l.val, v)
			maxV, maxM = math.Max(maxV, v), math.Max(maxM, m)
		}
		if len(l.val) > 0 {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	plotW, plotH := width-padL-padR, height-padT-padB
	step, top := axis("bytes", maxV)
	xOf := func(m float64) float64 { return padL + plotW*m/maxM }
	yOf := func(v float64) float64 { return padT + plotH - plotH*v/top }

	var sb strings.Builder
	names := make([]string, len(lines))
	for i, l := range lines {
		names[i] = l.engine
	}
	writeHeader(&sb, width, height, "Resident memory during ingest", "every engine on the same clock, minutes since its ingest started, lower is better", names)
	for v := 0.0; v <= top+step/2; v += step {
		fmt.Fprintf(&sb, `<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"/><text x="%.0f" y="%.1f" font-size="11" fill="%s" text-anchor="end">%s</text>`+"\n", padL, yOf(v), padL+plotW, yOf(v), svgGrid, padL-10, yOf(v)+4, svgMuted, fmtAxis("bytes", v))
	}
	acc := 0.0
	if ing := b.firstIngest(); ing != nil {
		for _, st := range ing.Steps {
			fmt.Fprintf(&sb, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="#cbd5e1" stroke-dasharray="4 4"/><text x="%.1f" y="%.0f" font-size="11" fill="%s">%dk series</text>`+"\n", xOf(acc), padT, xOf(acc), padT+plotH, xOf(acc)+6, padT+14, svgMuted, st.Series/1000)
			acc += st.DurationSeconds / 60
		}
	}
	for m := 0.0; m <= maxM; m += 5 {
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.0f" font-size="11" fill="%s" text-anchor="middle">%.0f min</text>`+"\n", xOf(m), padT+plotH+22, svgMuted, m)
	}
	for _, l := range lines {
		var pts []string
		for i := range l.val {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", xOf(l.min[i]), yOf(l.val[i])))
		}
		fmt.Fprintf(&sb, `<polyline fill="none" stroke="%s" stroke-width="2.2" stroke-linejoin="round" points="%s"/>`+"\n", styleOf(l.engine).color, strings.Join(pts, " "))
		last := len(l.val) - 1
		fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"/><text x="%.1f" y="%.1f" font-size="11" font-weight="600" fill="%s">%s</text>`+"\n", xOf(l.min[last]), yOf(l.val[last]), styleOf(l.engine).color, xOf(l.min[last])+8, yOf(l.val[last])+4, styleOf(l.engine).color, fmtShort("bytes", l.val[last]))
	}
	sb.WriteString("</svg>\n")
	return sb.String()
}

func (b *Bundle) firstIngest() *results.IngestResult {
	if len(b.Ingests) == 0 {
		return nil
	}
	return b.Ingests[0]
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
