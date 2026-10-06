package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xdearboy/prom-loadgen/results"
	"github.com/xdearboy/prom-loadgen/stats"
)

func TestLoadAndRenderFullRun(t *testing.T) {
	root := t.TempDir()
	runID := "20260101T000000Z"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	meta := results.RunMeta{
		RunID: runID,
		Node:  results.NodeInfo{Name: "bench-node", CPUCores: 8, CPUModel: "test cpu", MemTotalBytes: 15 << 30},
	}
	stepStart := func(i int) time.Time { return base.Add(time.Duration(i) * 300 * time.Second) }
	meta.Engines = []results.EngineMeta{
		{Name: "prompp-0815", Image: "mirror.gcr.io/prompp/prompp:0.8.15", Version: "0.8.15", MemBytes: 6 << 30, Notes: "upstream prometheus 2.55.1"},
		{Name: "prom-3150", Image: "quay.io/prometheus/prometheus:v3.15.0", Version: "3.15.0", MemBytes: 6 << 30},
	}
	if err := results.WriteJSON(results.ArtifactPath(root, runID, results.KindMeta, ""), meta); err != nil {
		t.Fatal(err)
	}

	rss := map[string]float64{
		"prompp-0815": 1.6e9,
		"prom-3150":   2.4e9,
	}
	for _, engine := range []string{"prompp-0815", "prom-3150"} {
		ingest := results.IngestResult{
			RunID: runID, Engine: engine, Target: "http://" + engine + ":9090",
			IntervalSeconds: 15, SeriesPlanned: 200000, BatchSeries: 5000, Workers: 4,
			EpochMs: base.UnixMilli(), TimestampsPinned: true,
			StartedAt: base, FinishedAt: base.Add(13 * time.Minute),
		}
		for i, series := range []int{50000, 200000} {
			ingest.Steps = append(ingest.Steps, results.IngestStep{
				Index: i, Series: series, DurationSeconds: 300, IntervalSeconds: 15,
				StartedAt: stepStart(i), FinishedAt: stepStart(i + 1), ElapsedSeconds: 300,
				Ticks: 20, Requests: 40, RequestsOK: 40, SamplesSent: int64(series * 20),
				AchievedSamplesPerSec: float64(series*20) / 300,
				RequestLatency:        stats.Summary{Count: 40, P50MS: 12, P99MS: 30},
				Head:                  results.HeadStats{NumSeries: int64(series), NumChunks: int64(series / 4)},
			})
		}
		if err := results.WriteJSON(results.ArtifactPath(root, runID, results.KindIngest, engine), ingest); err != nil {
			t.Fatal(err)
		}

		for _, phase := range []string{"ingest", "query"} {
			steps := 2
			if phase == "query" {
				steps = 0
			}
			samples := make([]results.Sample, 0, steps*20)
			for step := 0; step < steps; step++ {
				for tick := 0; tick < 20; tick++ {
					samples = append(samples, results.Sample{
						TS:     stepStart(step).Add(time.Duration(tick) * 5 * time.Second),
						RunID:  runID,
						Engine: engine,
						Phase:  phase,
						Self: map[string]float64{
							"process_resident_memory_bytes": rss[engine] + float64(tick)*1e6,
							"process_cpu_seconds_total":     float64(tick) * 0.5,
							"go_memstats_heap_alloc_bytes":  rss[engine] / 2,
							"go_memstats_sys_bytes":         rss[engine],
							"go_goroutines":                 120,
						},
						Cadvisor: map[string]float64{
							"engine_container_memory_working_set_bytes": rss[engine] * 1.1,
							"engine_container_cpu_usage_seconds_total":  float64(tick) * 0.45,
							"engine_container_oom_events_total":         0,
						},
					})
				}
			}
			if len(samples) == 0 {
				continue
			}
			var body strings.Builder
			for _, s := range samples {
				line, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				body.Write(line)
				body.WriteByte('\n')
			}
			path := results.SamplesPath(root, runID, engine, phase)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		var diskBody strings.Builder
		for i, series := range []int{50000, 200000} {
			line, err := json.Marshal(results.DiskSample{
				TS: stepStart(i + 1), RunID: runID, Engine: engine,
				DataDirBytes: int64(series) * 180, WALBytes: int64(series) * 20, BlocksBytes: int64(series) * 120,
			})
			if err != nil {
				t.Fatal(err)
			}
			diskBody.Write(line)
			diskBody.WriteByte('\n')
		}
		diskPath := results.DiskSamplesPath(root, runID, engine)
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(diskPath, []byte(diskBody.String()), 0o644); err != nil {
			t.Fatal(err)
		}

		for _, concurrency := range []int{1, 4} {
			q := results.QueryResult{
				RunID: runID, Engine: engine, Target: "http://" + engine + ":9090",
				Suite: "core", Concurrency: concurrency, DurationSec: 180, Phase: "query",
				StartedAt: base, FinishedAt: base.Add(3 * time.Minute),
			}
			for _, name := range []string{"rate_cpu", "topk_pods", "histogram_quantile"} {
				q.Queries = append(q.Queries, results.QueryMetric{
					Name: name, Category: "core", Type: "range",
					Requests: int64(60 * concurrency), Series: 500, Points: 30000,
					Latency: stats.Summary{Count: int64(60 * concurrency), P50MS: float64(10 * concurrency), P95MS: float64(25 * concurrency), P99MS: float64(40 * concurrency)},
				})
			}
			if err := results.WriteJSON(results.ArtifactPath(root, runID, results.KindQuery, engine), q); err != nil {
				t.Fatal(err)
			}
		}

		dump := results.DumpResult{RunID: runID, Engine: engine, Suite: "core", EpochMs: base.UnixMilli(), GeneratedAt: base}
		for _, name := range []string{"rate_cpu", "topk_pods", "histogram_quantile"} {
			sum := "digest-for-" + name
			dump.Entries = append(dump.Entries, results.DumpEntry{
				Name: name, Type: "range", SHA256: sum, Series: 500, Points: 30000, Elapsed: 0.4,
			})
		}
		if err := results.WriteJSON(results.ArtifactPath(root, runID, results.KindDump, engine), dump); err != nil {
			t.Fatal(err)
		}
	}

	bundle, err := Load(root, runID)
	if err != nil {
		t.Fatal(err)
	}
	bundle.AttachResources()

	engines := bundle.Engines()
	if len(engines) != 2 || engines[0] != "prom-3150" || engines[1] != "prompp-0815" {
		t.Fatalf("unexpected engines %v", engines)
	}

	step := bundle.step("prompp-0815", 200000)
	if step == nil || step.Resources == nil {
		t.Fatal("step resources missing")
	}
	if step.Resources.RSSBytesPerSeries <= 0 {
		t.Fatalf("rss per series not computed: %+v", step.Resources)
	}
	if step.Resources.DiskBytesPerSeries <= 0 {
		t.Fatalf("disk per series not computed: %+v", step.Resources)
	}
	if step.Resources.DataDirBytes != 200000*180 {
		t.Fatalf("disk sample not attached: %+v", step.Resources)
	}
	if step.Resources.CPUCoresAvg <= 0 {
		t.Fatalf("cpu not computed: %+v", step.Resources)
	}

	markdown := string(bundle.Markdown())
	for _, want := range []string{
		"# Prom++ versus Prometheus",
		"prompp-0815",
		"prom-3150",
		"mirror.gcr.io/prompp/prompp:0.8.15",
		"rss/series",
		"disk/series",
		"Result equality",
		"identical",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q", want)
		}
	}

	chartsDir := filepath.Join(root, runID, "charts")
	if err := bundle.WriteCharts(chartsDir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no charts written")
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(chartsDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(body), "<svg") {
			t.Fatalf("%s is not an svg", e.Name())
		}
		if !strings.Contains(string(body), "</svg>") {
			t.Fatalf("%s is truncated", e.Name())
		}
	}
}

func TestLoadRejectsEmptyRun(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(results.RunDir(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "empty"); err == nil {
		t.Fatal("expected an error for a run without artifacts")
	}
}

func TestSummarizeStepSurvivesMissingRSS(t *testing.T) {
	samples := []results.Sample{
		{TS: time.Now(), Self: map[string]float64{"go_goroutines": 7}, Cadvisor: map[string]float64{}},
	}
	out := summarizeStep(samples, 1000)
	if out == nil {
		t.Fatal("expected resources")
	}
	if out.RSSBytesPerSeries != 0 {
		t.Fatalf("want zero rss per series, got %v", out.RSSBytesPerSeries)
	}
	if out.GoroutinesAvg != 7 {
		t.Fatalf("want goroutines 7, got %v", out.GoroutinesAvg)
	}
}

func TestArtifactVariantsMatchRunnerNames(t *testing.T) {
	root := t.TempDir()
	runID := "run1"
	dir := results.EngineDir(root, runID, "prompp-0815")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"query-core-c1.json",
		"query-core-c4.json",
		"query-heavy-c16.json",
		"dump-core.json",
		"dump-heavy.json",
		"ingest.json",
		"flags.json",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"status":"success"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	queries, err := results.ListArtifacts(root, runID, results.KindQuery)
	if err != nil {
		t.Fatalf("list query artifacts: %v", err)
	}
	if len(queries) != 3 {
		t.Fatalf("want 3 query artifacts, got %d: %v", len(queries), queries)
	}
	for _, want := range []string{"query-core-c1.json", "query-core-c4.json", "query-heavy-c16.json"} {
		found := false
		for _, p := range queries {
			if filepath.Base(p) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("query artifact %s not listed", want)
		}
	}

	dumps, err := results.ListArtifacts(root, runID, results.KindDump)
	if err != nil {
		t.Fatalf("list dump artifacts: %v", err)
	}
	if len(dumps) != 2 {
		t.Fatalf("want 2 dump artifacts, got %d: %v", len(dumps), dumps)
	}

	ingests, err := results.ListArtifacts(root, runID, results.KindIngest)
	if err != nil {
		t.Fatalf("list ingest artifacts: %v", err)
	}
	if len(ingests) != 1 || filepath.Base(ingests[0]) != "ingest.json" {
		t.Fatalf("want exactly ingest.json, got %v", ingests)
	}
}

func TestDumpPrefersWidestSuite(t *testing.T) {
	b := &Bundle{Dumps: []*results.DumpResult{
		{Engine: "prompp-0815", Suite: "core", Entries: []results.DumpEntry{{Name: "sum"}}},
		{Engine: "prompp-0815", Suite: "heavy", Entries: []results.DumpEntry{{Name: "sum"}, {Name: "regex_name"}}},
		{Engine: "prom-3150", Suite: "core", Entries: []results.DumpEntry{{Name: "sum"}}},
	}}
	got := b.Dump("prompp-0815")
	if got == nil || got.Suite != "heavy" {
		t.Fatalf("want the heavy dump, got %+v", got)
	}
	if got := b.Dump("prom-2551"); got != nil {
		t.Fatalf("want nil for an engine without dumps, got %+v", got)
	}
}
