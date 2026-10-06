package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/xdearboy/prom-loadgen/results"
)

const baselineEngine = "prom-3150"

func (b *Bundle) summary() *chart {
	levels := b.seriesLevels()
	if len(levels) == 0 {
		return nil
	}
	engines := b.chartEngines()
	last := levels[len(levels)-1]
	type row struct {
		label string
		unit  string
		get   func(engine string) float64
	}
	stepValue := func(f func(*results.StepResources) float64) func(string) float64 {
		return func(e string) float64 {
			if st := b.step(e, last); st != nil && st.Resources != nil {
				return f(st.Resources)
			}
			return 0
		}
	}
	rows := []row{
		{"Memory, RSS average", "bytes", stepValue(func(r *results.StepResources) float64 { return r.RSSSelfAvgBytes })},
		{"Working set average", "bytes", stepValue(func(r *results.StepResources) float64 { return r.WorkingSetAvgBytes })},
		{"CPU while ingesting", "cores", stepValue(func(r *results.StepResources) float64 { return r.CPUCoresAvg })},
		{"Data directory", "bytes", stepValue(func(r *results.StepResources) float64 { return float64(r.DataDirBytes) })},
	}
	for _, c := range b.concurrencyLevels() {
		rows = append(rows, row{queryLabel(c), "ms", func(e string) float64 {
			for _, res := range b.QueriesFor(e) {
				if res.Concurrency == c {
					return geomeanLatency(res, func(q results.QueryMetric) float64 { return q.Latency.P50MS })
				}
			}
			return 0
		}})
	}

	out := &chart{
		title:    "Compared with Prometheus 3.15.0",
		subtitle: fmt.Sprintf("%dk series, Prometheus 3.15.0 is 100%%, lower is better, query latency is the geometric mean of the median", last/1000),
		unit:     "pct",
		ref:      100,
	}
	for _, r := range rows {
		base := r.get(baselineEngine)
		if base <= 0 {
			continue
		}
		var group []bar
		for _, e := range engines {
			v := r.get(e)
			if v <= 0 {
				continue
			}
			pct := v / base * 100
			group = append(group, bar{engine: e, value: pct, text: fmt.Sprintf("%s, %.0f%%", fmtShort(r.unit, v), pct)})
		}
		out.groups = append(out.groups, group)
		out.labels = append(out.labels, r.label)
	}
	if len(out.groups) == 0 {
		return nil
	}
	return out
}

func queryLabel(clients int) string {
	if clients == 1 {
		return "Query latency, 1 client"
	}
	return fmt.Sprintf("Query latency, %d clients", clients)
}

func (b *Bundle) latencyScaling() string {
	levels := b.concurrencyLevels()
	if len(levels) < 2 {
		return ""
	}
	type point struct{ x, v float64 }
	lines := map[string][]point{}
	maxV := 0.0
	var engines []string
	for _, e := range b.chartEngines() {
		for i, c := range levels {
			for _, res := range b.QueriesFor(e) {
				if res.Concurrency != c {
					continue
				}
				if v := geomeanLatency(res, func(q results.QueryMetric) float64 { return q.Latency.P50MS }); v > 0 {
					lines[e] = append(lines[e], point{float64(i), v})
					maxV = math.Max(maxV, v)
				}
			}
		}
		if len(lines[e]) > 0 {
			engines = append(engines, e)
		}
	}
	if maxV == 0 {
		return ""
	}
	const width, height, padL, padR, padT, padB = 980.0, 420.0, 78.0, 110.0, 84.0, 52.0
	plotW, plotH := width-padL-padR, height-padT-padB
	step, top := axis("ms", maxV)
	xOf := func(i float64) float64 { return padL + plotW*(i+0.5)/float64(len(levels)) }
	yOf := func(v float64) float64 { return padT + plotH - plotH*v/top }

	var sb strings.Builder
	writeHeader(&sb, width, height, "Latency under concurrency", "geometric mean of the median over all queries, lower is better", engines)
	for v := 0.0; v <= top+step/2; v += step {
		fmt.Fprintf(&sb, `<line x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"/><text x="%.0f" y="%.1f" font-size="11" fill="%s" text-anchor="end">%s</text>`+"\n", padL, yOf(v), padL+plotW, yOf(v), svgGrid, padL-10, yOf(v)+4, svgMuted, fmtAxis("ms", v))
	}
	for i, c := range levels {
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.0f" font-size="13" font-weight="600" fill="#1e293b" text-anchor="middle">%d clients</text>`+"\n", xOf(float64(i)), padT+plotH+26, c)
	}
	for _, e := range engines {
		color := styleOf(e).color
		var pts []string
		for _, p := range lines[e] {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", xOf(p.x), yOf(p.v)))
		}
		fmt.Fprintf(&sb, `<polyline fill="none" stroke="%s" stroke-width="2.4" stroke-linejoin="round" points="%s"/>`+"\n", color, strings.Join(pts, " "))
		for _, p := range lines[e] {
			fmt.Fprintf(&sb, `<circle cx="%.1f" cy="%.1f" r="4" fill="%s"/>`+"\n", xOf(p.x), yOf(p.v), color)
		}
	}
	type endLabel struct {
		engine string
		y      float64
		v      float64
	}
	var ends []endLabel
	for _, e := range engines {
		p := lines[e][len(lines[e])-1]
		ends = append(ends, endLabel{e, yOf(p.v), p.v})
	}
	sort.Slice(ends, func(i, j int) bool { return ends[i].y < ends[j].y })
	for i := 1; i < len(ends); i++ {
		ends[i].y = math.Max(ends[i].y, ends[i-1].y+14)
	}
	for _, l := range ends {
		fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" font-size="11" font-weight="600" fill="%s">%s</text>`+"\n", xOf(float64(len(levels)-1))+9, l.y+4, styleOf(l.engine).color, fmtShort("ms", l.v))
	}
	sb.WriteString("</svg>\n")
	return sb.String()
}
