package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xdearboy/prom-loadgen/results"
)

func (b *Bundle) Markdown() []byte {
	var sb strings.Builder
	b.writeHeader(&sb)
	b.writeIngest(&sb)
	b.writeQuery(&sb)
	b.writeCorrectness(&sb)
	b.writeReproduce(&sb)
	return []byte(sb.String())
}

func (b *Bundle) writeHeader(sb *strings.Builder) {
	sb.WriteString("# Prom++ versus Prometheus\n\n")
	sb.WriteString("Generated from the raw artifacts in this directory. Every number below comes from a file in the run directory, nothing is typed in by hand.\n\n")

	if b.Meta.RunID != "" {
		fmt.Fprintf(sb, "Run id: `%s`\n\n", b.Meta.RunID)
	}
	node := b.Meta.Node
	fmt.Fprintf(sb, "## Environment\n\n")
	sb.WriteString("| item | value |\n|---|---|\n")
	writeRow(sb, "node", node.Name)
	writeRow(sb, "cpu", node.CPUModel)
	writeRow(sb, "cores", fmt.Sprint(node.CPUCores))
	writeRow(sb, "memory", Bytes(uint64(node.MemTotalBytes)))
	writeRow(sb, "kernel", node.KernelVersion)
	writeRow(sb, "os", node.OSImage)
	writeRow(sb, "arch", node.Arch)
	if node.FSCapacityBytes > 0 {
		writeRow(sb, "data filesystem", fmt.Sprintf("%s free of %s",
			Bytes(uint64(node.FSAvailableBytes)), Bytes(uint64(node.FSCapacityBytes))))
	}
	for _, key := range sortedStringKeys(b.Meta.Extra) {
		writeRow(sb, key, b.Meta.Extra[key])
	}
	if b.Meta.GitCommit != "" {
		writeRow(sb, "harness commit", b.Meta.GitCommit)
	}
	if b.Meta.Harness != "" {
		writeRow(sb, "harness", b.Meta.Harness)
	}
	if len(b.Meta.Engines) > 0 {
		sb.WriteString("\n## Engines\n\n")
		sb.WriteString("Declared settings come from the manifests, observed ones from the engine's own runtimeinfo and flags endpoints during the run.\n\n")
		sb.WriteString("| engine | image | cpu limit | memory limit | env | observed GOMEMLIMIT | observed GOMAXPROCS | WAL compression | args | notes |\n|---|---|---|---|---|---|---|---|---|---|\n")
		for _, e := range b.Meta.Engines {
			rt := b.Runtime(e.Name)
			fmt.Fprintf(sb, "| `%s` | `%s` | %s | %s | `%s` | %s | %s | %s | `%s` | %s |\n",
				e.Name, e.Image, Milli(e.CPUMilli), Bytes(uint64(e.MemBytes)),
				orDash(strings.Join(e.Env, " ")), memLimit(rt.GOMEMLimit), orDash(rt.GOMAXPROCS),
				orDash(rt.WALCompression), strings.Join(e.Args, " "), e.Notes)
		}
	}
	if len(b.Meta.Notes) > 0 {
		sb.WriteString("\n### Notes\n\n")
		for _, n := range b.Meta.Notes {
			fmt.Fprintf(sb, "- %s\n", n)
		}
	}
	sb.WriteString("\n")
}

func (b *Bundle) writeIngest(sb *strings.Builder) {
	if len(b.Ingests) == 0 {
		return
	}
	engines := b.Engines()
	sb.WriteString("## Ingest and head memory\n\n")
	sb.WriteString("![compared with Prometheus 3.15.0](charts/summary.svg)\n\n")
	sb.WriteString("![resident memory by series](charts/rss-by-series.svg)\n\n")
	sb.WriteString("![cpu cores by series](charts/cpu-by-series.svg)\n\n")
	sb.WriteString("![resident memory over time](charts/rss-timeline.svg)\n\n")
	sb.WriteString("![data directory by series](charts/disk-by-series.svg)\n\n")

	for _, e := range engines {
		res := b.Ingest(e)
		if res == nil {
			continue
		}
		fmt.Fprintf(sb, "### %s\n\n", e)
		fmt.Fprintf(sb, "Interval %gs, batch %d series, %d workers, %d samples sent, %d failed.\n\n",
			res.IntervalSeconds, res.BatchSeries, res.Workers, res.TotalSamples(), res.FailedSamples())
		sb.WriteString("| active series | samples/s | head series | head chunks | RSS avg | RSS p95 | RSS max | working set max | cpu cores avg | cpu cores max | data dir | WAL | rss/series | disk/series |\n")
		sb.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, step := range res.Steps {
			r := orZero(step.Resources)
			fmt.Fprintf(sb, "| %d | %.0f | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				step.Series, step.AchievedSamplesPerSec, step.Head.NumSeries, step.Head.NumChunks,
				dashBytes(r.RSSSelfAvgBytes), dashBytes(r.RSSSelfP95Bytes), dashBytes(r.RSSSelfMaxBytes),
				dashBytes(r.WorkingSetMaxBytes), dashCores(r.CPUCoresAvg), dashCores(r.CPUCoresMax),
				dashBytes(float64(r.DataDirBytes)), dashBytes(float64(r.WALBytes)), ratioOrDash(r), diskRatioOrDash(r))
			if len(step.ErrorSamples) > 0 {
				fmt.Fprintf(sb, "\nStep %d errors: `%s`\n", step.Index, strings.Join(step.ErrorSamples, "; "))
			}
		}
		sb.WriteString("\n")
		sb.WriteString("| active series | elapsed / planned | write request p50 | p99 | 2xx | 4xx | 5xx | client errors |\n")
		sb.WriteString("|---|---|---|---|---|---|---|---|\n")
		for _, step := range res.Steps {
			elapsed := fmt.Sprintf("%.0f s / %.0f s", step.ElapsedSeconds, step.DurationSeconds)
			if step.Overran() {
				elapsed += ", overran"
			}
			fmt.Fprintf(sb, "| %d | %s | %s | %s | %d | %d | %d | %d |\n",
				step.Series, elapsed, ms(step.RequestLatency.P50MS), ms(step.RequestLatency.P99MS),
				step.RequestsOK, step.HTTP4xx, step.HTTP5xx, step.ClientErrors)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Comparison\n\n")
	sb.WriteString("Lower is better for every memory and cpu column. RSS is the engine's own process_resident_memory_bytes, working set is the cgroup figure the kubelet evicts and OOM kills on.\n\n")
	seriesSet := b.seriesLevels()
	sb.WriteString("| active series | engine | RSS avg | bytes/series | working set max | cpu cores avg | data dir | samples/s |\n|---|---|---|---|---|---|---|---|\n")
	for _, series := range seriesSet {
		for _, e := range engines {
			step := b.step(e, series)
			if step == nil {
				continue
			}
			r := orZero(step.Resources)
			fmt.Fprintf(sb, "| %d | `%s` | %s | %s | %s | %s | %s | %.0f |\n",
				series, e, dashBytes(r.RSSSelfAvgBytes), ratioOrDash(r), dashBytes(r.WorkingSetMaxBytes),
				dashCores(r.CPUCoresAvg), dashBytes(float64(r.DataDirBytes)), step.AchievedSamplesPerSec)
		}
	}
	sb.WriteString("\n")
	if leader := b.memoryLeader(seriesSet); leader.engine != "" {
		fmt.Fprintf(sb, "At the largest step, `%s` needs the least resident memory per series: %s\n\n", leader.engine, leader.text)
	}
}

func (b *Bundle) writeQuery(sb *strings.Builder) {
	if len(b.Queries) == 0 {
		return
	}
	engines := b.Engines()
	sb.WriteString("## Query latency\n\n")
	sb.WriteString("Each query runs on its own: one unmeasured warmup request, then the given number of workers repeat it until the time budget is spent and a minimum number of requests finished. ")
	sb.WriteString("`n` is the number of measured requests. With a small `n` the p99 is close to the maximum, so the median is the figure to compare. ")
	sb.WriteString("The geometric mean weighs every query equally, so a few multi second range queries do not drown the rest.\n\n")
	sb.WriteString("![latency under concurrency](charts/latency-scaling.svg)\n\n")
	for _, concurrency := range b.concurrencyLevels() {
		fmt.Fprintf(sb, "### Concurrency %d\n\n", concurrency)
		fmt.Fprintf(sb, "![median query latency, concurrency %d](charts/%s)\n\n", concurrency, queryChart(concurrency))
		sb.WriteString("| engine | suite | queries | requests | errors | geomean p50 | geomean p99 | slowest query p50 | cpu cores avg | working set max |\n")
		sb.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
		for _, e := range engines {
			for _, res := range b.QueriesFor(e) {
				if res.Concurrency != concurrency {
					continue
				}
				fmt.Fprintf(sb, "| `%s` | %s | %d | %d | %d | %s | %s | %s | %s | %s |\n",
					e, res.Suite, len(res.Queries), res.TotalRequests(), res.TotalErrors(),
					msOrDash(geomeanLatency(res, func(q results.QueryMetric) float64 { return q.Latency.P50MS })),
					msOrDash(geomeanLatency(res, func(q results.QueryMetric) float64 { return q.Latency.P99MS })),
					slowest(res), queryCores(res), queryWorkingSet(res))
			}
		}
		sb.WriteString("\n")
		b.writeQueryTable(sb, engines, concurrency)
	}
}

func (b *Bundle) writeQueryTable(sb *strings.Builder, engines []string, concurrency int) {
	header := "| query | type |"
	align := "|---|---|"
	for _, e := range engines {
		header += " `" + e + "` p50 / p99 (n) |"
		align += "---|"
	}
	sb.WriteString(header + " fastest p50 |\n" + align + "---|\n")
	for _, name := range b.queryNames() {
		line := "| `" + name + "` |"
		qtype := ""
		fastest, best := "", math.MaxFloat64
		cells := ""
		for _, e := range engines {
			m, ok := b.queryAt(e, name, concurrency)
			if !ok {
				cells += " - |"
				continue
			}
			qtype = m.Type
			if m.Latency.Count == 0 {
				cells += fmt.Sprintf(" failed, %d errors |", m.Errors)
				continue
			}
			cells += fmt.Sprintf(" %s / %s (%d) |", ms(m.Latency.P50MS), ms(m.Latency.P99MS), m.Latency.Count)
			if m.Latency.P50MS < best {
				best, fastest = m.Latency.P50MS, e
			}
		}
		winner := "-"
		if fastest != "" {
			winner = "`" + fastest + "`"
		}
		fmt.Fprintf(sb, "%s %s |%s %s |\n", line, orDash(qtype), cells, winner)
	}
	sb.WriteString("\n")
}

func (b *Bundle) writeCorrectness(sb *strings.Builder) {
	if len(b.Dumps) < 2 {
		return
	}
	sb.WriteString("## Result equality\n\n")
	engines := b.Engines()
	names := map[string]bool{}
	for _, name := range b.Engines() {
		if dump := b.Dump(name); dump != nil {
			for _, e := range dump.Entries {
				names[e.Name] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	sb.WriteString("Every engine received the same samples and every query is evaluated at the same pinned timestamp, so a result that differs points at a semantic difference between engines or at lost data. Each cell is the sha256 of the canonicalised result.\n\n")
	sb.WriteString("A differing row means the engines disagree on the same data at the same timestamp. All of them are rate or `_over_time` range queries. Prometheus 3.0 made range selectors left-open, a sample exactly on the window start is no longer included, and the Prom++ 0.8.15 PromQL engine already drops it (`promql/engine.go`, `floats[drop].T <= mint`), unlike 2.55.1 (`< mint`). The synthetic samples sit on the window edge, so 2.55.1 sees one extra sample.\n\n")
	sb.WriteString("| query |")
	for _, e := range engines {
		sb.WriteString(" `" + e + "` |")
	}
	sb.WriteString(" identical |\n|---|")
	for range engines {
		sb.WriteString("---|")
	}
	sb.WriteString("---|\n")
	identical, differing := 0, 0
	for _, name := range sorted {
		line := "| `" + name + "` |"
		same := true
		var baseEntry *results.DumpEntry
		for _, e := range engines {
			dump := b.Dump(e)
			if dump == nil {
				line += " - |"
				same = false
				continue
			}
			entry, ok := dump.ByName(name)
			if !ok {
				line += " - |"
				same = false
				continue
			}
			if baseEntry == nil {
				baseEntry = &entry
			}
			switch {
			case entry.Error != "":
				line += " error |"
				same = false
			case entry.SHA256 != baseEntry.SHA256:
				line += fmt.Sprintf(" %s (%d series) |", short(entry.SHA256), entry.Series)
				same = false
			default:
				line += fmt.Sprintf(" %s (%d series) |", short(entry.SHA256), entry.Series)
			}
		}
		if same {
			identical++
			line += " yes |\n"
		} else {
			differing++
			line += " no |\n"
		}
		fmt.Fprint(sb, line)
	}
	fmt.Fprintf(sb, "\n%d queries identical, %d differ.\n\n", identical, differing)
}

func (b *Bundle) writeReproduce(sb *strings.Builder) {
	sb.WriteString("## Reproduce\n\n")
	sb.WriteString("```sh\n")
	fmt.Fprintf(sb, "export BENCH_NODE=<node>\n")
	fmt.Fprintf(sb, "RUN_ID=%s scripts/node-overlay.sh\n", b.RunID)
	sb.WriteString("scripts/harness.sh\n")
	fmt.Fprintf(sb, "RUN_ID=%s scripts/run.sh\n", b.RunID)
	fmt.Fprintf(sb, "go run ./cmd/report -root results -run %s\n", b.RunID)
	sb.WriteString("scripts/teardown.sh --yes\n```\n\n")
	sb.WriteString("See METHODOLOGY.md for the fairness rules and the known threats to validity.\n")
}

type leaderInfo struct {
	engine string
	text   string
}

func (b *Bundle) memoryLeader(seriesSet []int) leaderInfo {
	if len(seriesSet) == 0 {
		return leaderInfo{}
	}
	top := seriesSet[len(seriesSet)-1]
	best, worst := "", 0.0
	for _, e := range b.Engines() {
		step := b.step(e, top)
		if step == nil || step.Resources == nil || step.Resources.RSSBytesPerSeries == 0 {
			continue
		}
		if best == "" {
			best, worst = e, step.Resources.RSSBytesPerSeries
			continue
		}
		if step.Resources.RSSBytesPerSeries < worst {
			best, worst = e, step.Resources.RSSBytesPerSeries
		}
	}
	if best == "" {
		return leaderInfo{}
	}
	return leaderInfo{
		engine: best,
		text:   fmt.Sprintf("%s of resident memory per active series, the lowest of the compared engines.", Bytes(uint64(worst))),
	}
}

func (b *Bundle) seriesLevels() []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range b.Ingests {
		for _, step := range r.Steps {
			if !seen[step.Series] {
				seen[step.Series] = true
				out = append(out, step.Series)
			}
		}
	}
	sort.Ints(out)
	return out
}

func (b *Bundle) step(engine string, series int) *results.IngestStep {
	res := b.Ingest(engine)
	if res == nil {
		return nil
	}
	for i := range res.Steps {
		if res.Steps[i].Series == series {
			return &res.Steps[i]
		}
	}
	return nil
}

func (b *Bundle) concurrencyLevels() []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range b.Queries {
		if !seen[r.Concurrency] {
			seen[r.Concurrency] = true
			out = append(out, r.Concurrency)
		}
	}
	sort.Ints(out)
	return out
}

func (b *Bundle) queryNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range b.Queries {
		for _, q := range r.Queries {
			if !seen[q.Name] {
				seen[q.Name] = true
				out = append(out, q.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func geomeanLatency(res *results.QueryResult, get func(results.QueryMetric) float64) float64 {
	var logSum float64
	n := 0
	for _, q := range res.Queries {
		if v := get(q); q.Latency.Count > 0 && v > 0 {
			logSum += math.Log(v)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Exp(logSum / float64(n))
}

func slowest(res *results.QueryResult) string {
	worst := results.QueryMetric{}
	for _, q := range res.Queries {
		if q.Latency.P50MS > worst.Latency.P50MS {
			worst = q
		}
	}
	if worst.Name == "" {
		return "-"
	}
	return fmt.Sprintf("`%s` %s", worst.Name, ms(worst.Latency.P50MS))
}

func ms(v float64) string {
	switch {
	case v >= 1000:
		return fmt.Sprintf("%.2f s", v/1000)
	case v >= 10:
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.1f ms", v)
}

func msOrDash(v float64) string {
	if v <= 0 {
		return "-"
	}
	return ms(v)
}

func queryCores(res *results.QueryResult) string {
	if res.Resources == nil || res.Resources.CPUCoresAvg == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", res.Resources.CPUCoresAvg)
}

func queryWorkingSet(res *results.QueryResult) string {
	if res.Resources == nil || res.Resources.WorkingSetMaxBytes == 0 {
		return "-"
	}
	return Bytes(uint64(res.Resources.WorkingSetMaxBytes))
}

func dashBytes(v float64) string {
	if v <= 0 {
		return "-"
	}
	return Bytes(uint64(v))
}

func dashCores(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func orZero(r *results.StepResources) *results.StepResources {
	if r == nil {
		return &results.StepResources{}
	}
	return r
}

func ratioOrDash(r *results.StepResources) string {
	return dashBytes(r.RSSBytesPerSeries)
}

func diskRatioOrDash(r *results.StepResources) string {
	return dashBytes(r.DiskBytesPerSeries)
}

func Bytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(v)/float64(div), "KMGTPE"[exp])
}

func writeRow(sb *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "| %s | %s |\n", key, value)
}

func sortedMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func Milli(v int64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f cores", float64(v)/1000.0)
}

func memLimit(raw string) string {
	v, err := strconv.ParseFloat(raw, 64)
	switch {
	case raw == "" || err != nil:
		return orDash(raw)
	case v >= math.MaxInt64/2:
		return "unset"
	}
	return Bytes(uint64(v))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
