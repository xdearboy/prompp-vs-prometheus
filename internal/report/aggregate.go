package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xdearboy/prom-loadgen/results"
)

type metric struct {
	title  string
	arg    int
	format func(float64) string
	values map[string][]float64
}

func newMetric(title string, format func(float64) string) *metric {
	return &metric{title: title, format: format, values: map[string][]float64{}}
}

func (m *metric) add(engine string, v float64) {
	if v > 0 {
		m.values[engine] = append(m.values[engine], v)
	}
}

func Aggregate(root string, runIDs []string, lang string) ([]byte, error) {
	var (
		rss  = newMetric("RSS at the last step", preciseBytes)
		ws   = newMetric("Working set at the last step", preciseBytes)
		cpu  = newMetric("Ingest cpu cores, average", func(v float64) string { return fmt.Sprintf("%.2f", v) })
		disk = newMetric("Data directory", preciseBytes)
		lat  = map[int]*metric{}
	)
	for _, id := range runIDs {
		b, err := Load(root, id)
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", id, err)
		}
		b.AttachResources()
		for _, engine := range b.Engines() {
			if ing := b.Ingest(engine); ing != nil && len(ing.Steps) > 0 {
				last := ing.Steps[len(ing.Steps)-1]
				if r := last.Resources; r != nil {
					rss.add(engine, r.RSSSelfAvgBytes)
					ws.add(engine, r.WorkingSetAvgBytes)
					cpu.add(engine, r.CPUCoresAvg)
					disk.add(engine, float64(r.DataDirBytes))
				}
			}
			for _, q := range b.QueriesFor(engine) {
				if lat[q.Concurrency] == nil {
					lat[q.Concurrency] = newMetric("Query geomean p50, concurrency %d", ms)
					lat[q.Concurrency].arg = q.Concurrency
				}
				lat[q.Concurrency].add(engine, geomeanLatency(q, func(m results.QueryMetric) float64 { return m.Latency.P50MS }))
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(switcher(lang, "AGGREGATE"))
	fmt.Fprintf(&sb, translate(lang, "# Aggregate of %d runs\n\n"), len(runIDs))
	sb.WriteString(translate(lang, "Each cell is the median over runs with the minimum and maximum, and the spread as the share of the median.\n\n"))
	all := []*metric{rss, ws, cpu, disk}
	var levels []int
	for c := range lat {
		levels = append(levels, c)
	}
	sort.Ints(levels)
	for _, c := range levels {
		all = append(all, lat[c])
	}
	for _, m := range all {
		m.write(&sb, lang)
	}
	return []byte(sb.String()), nil
}

func (m *metric) write(sb *strings.Builder, lang string) {
	title := translate(lang, m.title)
	if strings.Contains(title, "%d") {
		title = fmt.Sprintf(title, m.arg)
	}
	fmt.Fprintf(sb, "## %s\n\n%s", title, translate(lang, "| engine | runs | median | min to max | spread |\n|---|---|---|---|---|\n"))
	for _, engine := range engineOrder {
		v := m.values[engine]
		if len(v) == 0 {
			continue
		}
		sort.Float64s(v)
		med := median(v)
		lo, hi := v[0], v[len(v)-1]
		fmt.Fprintf(sb, "| %s | %d | %s | %s %s %s | %.1f%% |\n",
			styleOf(engine).title, len(v), m.format(med), m.format(lo), translate(lang, "to"), m.format(hi), (hi-lo)/med*100)
	}
	sb.WriteString("\n")
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func preciseBytes(v float64) string {
	if v >= 1<<30 {
		return fmt.Sprintf("%.2f GiB", v/(1<<30))
	}
	return Bytes(uint64(v))
}
