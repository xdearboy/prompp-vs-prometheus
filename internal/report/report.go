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

type Bundle struct {
	Lang    string
	Root    string
	RunID   string
	Meta    results.RunMeta
	Ingests []*results.IngestResult
	Queries []*results.QueryResult
	Dumps   []*results.DumpResult
	Samples map[string][]results.Sample
	Disks   map[string]*results.DiskArtifact
}

func Load(root, runID string) (*Bundle, error) {
	b := &Bundle{Root: root, RunID: runID, Samples: map[string][]results.Sample{}, Disks: map[string]*results.DiskArtifact{}}
	metaPath := results.ArtifactPath(root, runID, results.KindMeta, "")
	if _, err := os.Stat(metaPath); err == nil {
		if err := results.ReadJSON(metaPath, &b.Meta); err != nil {
			return nil, err
		}
	}
	ingestPaths, err := results.ListArtifacts(root, runID, results.KindIngest)
	if err == nil {
		for _, path := range ingestPaths {
			var res results.IngestResult
			if err := results.ReadJSON(path, &res); err != nil {
				return nil, err
			}
			b.Ingests = append(b.Ingests, &res)
		}
	}
	queryPaths, err := results.ListArtifacts(root, runID, results.KindQuery)
	if err == nil {
		for _, path := range queryPaths {
			var res results.QueryResult
			if err := results.ReadJSON(path, &res); err != nil {
				return nil, err
			}
			b.Queries = append(b.Queries, &res)
		}
	}
	dumpPaths, err := results.ListArtifacts(root, runID, results.KindDump)
	if err == nil {
		for _, path := range dumpPaths {
			var res results.DumpResult
			if err := results.ReadJSON(path, &res); err != nil {
				return nil, err
			}
			b.Dumps = append(b.Dumps, &res)
		}
	}
	engineNames, err := results.ListEngines(root, runID)
	if err == nil {
		for _, engine := range engineNames {
			path := results.DiskSamplesPath(root, runID, engine)
			samples, err := results.ReadDiskSamples(path)
			if err != nil {
				continue
			}
			b.Disks[engine] = &results.DiskArtifact{RunID: runID, Engine: engine, Samples: samples}
		}
	}
	if len(b.Ingests) == 0 && len(b.Queries) == 0 && len(b.Dumps) == 0 {
		return nil, fmt.Errorf("no artifacts for run %s in %s", runID, results.RunDir(root, runID))
	}
	for _, res := range b.Ingests {
		for _, phase := range []string{"ingest"} {
			path := results.SamplesPath(root, runID, res.Engine, phase)
			if samples, err := results.ReadSamples(path); err == nil {
				b.Samples[res.Engine+"/"+phase] = samples
			}
		}
	}
	for _, res := range b.Queries {
		path := results.SamplesPath(root, runID, res.Engine, res.Phase)
		if samples, err := results.ReadSamples(path); err == nil {
			b.Samples[res.Engine+"/"+res.Phase] = samples
		}
	}
	return b, nil
}

func (b *Bundle) Engines() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range b.Ingests {
		if !seen[r.Engine] {
			seen[r.Engine] = true
			out = append(out, r.Engine)
		}
	}
	for _, r := range b.Queries {
		if !seen[r.Engine] {
			seen[r.Engine] = true
			out = append(out, r.Engine)
		}
	}
	for _, r := range b.Dumps {
		if !seen[r.Engine] {
			seen[r.Engine] = true
			out = append(out, r.Engine)
		}
	}
	sort.Strings(out)
	return out
}

type Runtime struct {
	GOMEMLimit     string
	GOMAXPROCS     string
	WALCompression string
}

func (b *Bundle) Runtime(engine string) Runtime {
	var out Runtime
	for _, key := range sortedSampleKeys(b.Samples) {
		if !strings.HasPrefix(key, engine+"/") {
			continue
		}
		for _, s := range b.Samples[key] {
			if out.GOMEMLimit == "" {
				out.GOMEMLimit = s.GOMEMLimit
			}
			if out.GOMAXPROCS == "" {
				out.GOMAXPROCS = s.GOMAXPROCS
			}
			if out.WALCompression == "" {
				out.WALCompression = s.WALCompression
			}
		}
	}
	return out
}

func sortedSampleKeys(m map[string][]results.Sample) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (b *Bundle) Ingest(engine string) *results.IngestResult {
	for _, r := range b.Ingests {
		if r.Engine == engine {
			return r
		}
	}
	return nil
}

func (b *Bundle) QueriesFor(engine string) []*results.QueryResult {
	var out []*results.QueryResult
	for _, r := range b.Queries {
		if r.Engine == engine {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Concurrency < out[j].Concurrency })
	return out
}

func (b *Bundle) Dump(engine string) *results.DumpResult {
	var best *results.DumpResult
	for _, r := range b.Dumps {
		if r.Engine != engine {
			continue
		}
		if best == nil || len(r.Entries) > len(best.Entries) {
			best = r
		}
	}
	return best
}

func (b *Bundle) AttachResources() {
	for _, res := range b.Ingests {
		samples := b.Samples[res.Engine+"/ingest"]
		for i := range res.Steps {
			step := &res.Steps[i]
			window := samplesForStep(samples, *step)
			if len(window) == 0 {
				continue
			}
			step.Resources = summarizeStep(window, step.Series)
			if disk, ok := b.Disks[res.Engine]; ok {
				if ds, ok := disk.At(step.FinishedAt); ok {
					step.Resources.DataDirBytes = ds.DataDirBytes
					step.Resources.WALBytes = ds.WALBytes
					step.Resources.BlocksBytes = ds.BlocksBytes
				}
			}
			if step.Resources != nil && step.Resources.DataDirBytes > 0 && step.Series > 0 {
				step.Resources.DiskBytesPerSeries = float64(step.Resources.DataDirBytes) / float64(step.Series)
			}
		}
	}
	for _, res := range b.Queries {
		samples := b.Samples[res.Engine+"/"+res.Phase]
		if len(samples) == 0 {
			continue
		}
		res.Resources = summarizeQuery(samples)
	}
}

func samplesForStep(samples []results.Sample, step results.IngestStep) []results.Sample {
	if step.StartedAt.IsZero() {
		return nil
	}
	var out []results.Sample
	for _, s := range samples {
		if !s.TS.Before(step.StartedAt) && !s.TS.After(step.FinishedAt) {
			out = append(out, s)
		}
	}
	return out
}

type series struct {
	ts  []time.Time
	val []float64
}

func seriesFor(samples []results.Sample, get func(results.Sample) (float64, bool)) series {
	out := series{}
	for _, s := range samples {
		if v, ok := get(s); ok {
			out.ts = append(out.ts, s.TS)
			out.val = append(out.val, v)
		}
	}
	return out
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func maxOf(values []float64) float64 {
	out := 0.0
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}

func quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(q*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func cores(samples []results.Sample, get func(results.Sample) float64) (avg, peak float64) {
	var points []float64
	for i := 1; i < len(samples); i++ {
		prev, cur := get(samples[i-1]), get(samples[i])
		dt := samples[i].TS.Sub(samples[i-1].TS).Seconds()
		if dt <= 0 || cur < prev {
			continue
		}
		points = append(points, (cur-prev)/dt)
	}
	if len(points) == 0 {
		return 0, 0
	}
	return mean(points), maxOf(points)
}

func summarizeStep(samples []results.Sample, series int) *results.StepResources {
	rss := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Self["process_resident_memory_bytes"]
		return v, ok
	})
	workingSet := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Cadvisor["engine_container_memory_working_set_bytes"]
		return v, ok
	})
	heap := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Self["go_memstats_heap_alloc_bytes"]
		return v, ok
	})
	sysMem := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Self["go_memstats_sys_bytes"]
		return v, ok
	})
	goroutines := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Self["go_goroutines"]
		return v, ok
	})

	cpuSelfAvg, cpuSelfMax := cores(samples, func(s results.Sample) float64 {
		return s.Self["process_cpu_seconds_total"]
	})
	cpuCadAvg, cpuCadMax := cores(samples, func(s results.Sample) float64 {
		return s.Cadvisor["engine_container_cpu_usage_seconds_total"]
	})
	cpuAvg, cpuMax := cpuSelfAvg, cpuSelfMax
	if cpuCadAvg > 0 {
		cpuAvg, cpuMax = cpuCadAvg, cpuCadMax
	}

	out := &results.StepResources{
		Samples:            len(samples),
		RSSSelfAvgBytes:    mean(rss.val),
		RSSSelfP95Bytes:    quantile(rss.val, 0.95),
		RSSSelfMaxBytes:    maxOf(rss.val),
		WorkingSetAvgBytes: mean(workingSet.val),
		WorkingSetMaxBytes: maxOf(workingSet.val),
		CPUCoresAvg:        cpuAvg,
		CPUCoresMax:        cpuMax,
		GoMemHeapAvgBytes:  mean(heap.val),
		GoMemSysAvgBytes:   mean(sysMem.val),
		GoroutinesAvg:      mean(goroutines.val),
	}
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		out.OOMKills = int64(last.Cadvisor["engine_container_oom_events_total"])
	}
	if n := len(rss.val); n > 0 && series > 0 {
		out.RSSBytesPerSeries = rss.val[n-1] / float64(series)
	}
	return out
}

func summarizeQuery(samples []results.Sample) *results.QueryResources {
	rss := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Self["process_resident_memory_bytes"]
		return v, ok
	})
	workingSet := seriesFor(samples, func(s results.Sample) (float64, bool) {
		v, ok := s.Cadvisor["engine_container_memory_working_set_bytes"]
		return v, ok
	})
	cpuAvg, cpuMax := cores(samples, func(s results.Sample) float64 {
		return s.Cadvisor["engine_container_cpu_usage_seconds_total"]
	})
	if cpuAvg == 0 {
		cpuAvg, cpuMax = cores(samples, func(s results.Sample) float64 {
			return s.Self["process_cpu_seconds_total"]
		})
	}
	return &results.QueryResources{
		Samples:            len(samples),
		CPUCoresAvg:        cpuAvg,
		CPUCoresMax:        cpuMax,
		RSSSelfAvgBytes:    mean(rss.val),
		WorkingSetAvgBytes: mean(workingSet.val),
		WorkingSetMaxBytes: maxOf(workingSet.val),
	}
}

func ResultsDir(root, runID string) string {
	return results.RunDir(root, runID)
}

func WriteFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
