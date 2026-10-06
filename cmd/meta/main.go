package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xdearboy/prom-loadgen/kube"
	"github.com/xdearboy/prom-loadgen/results"
	"github.com/xdearboy/prompp-vs-prometheus/internal/engines"
)

type nodeObject struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Capacity    map[string]string `json:"capacity"`
		Allocatable map[string]string `json:"allocatable"`
		NodeInfo    struct {
			KubeletVersion          string `json:"kubeletVersion"`
			OSImage                 string `json:"osImage"`
			KernelVersion           string `json:"kernelVersion"`
			Architecture            string `json:"architecture"`
			ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
			OperatingSystem         string `json:"operatingSystem"`
		} `json:"nodeInfo"`
	} `json:"status"`
}

func collectMeta(node, runID string, names []string, notes []string) (results.RunMeta, error) {
	meta := results.RunMeta{
		RunID:     runID,
		StartedAt: time.Now().UTC(),
		Harness:   "harness/" + version(),
		Node:      results.NodeInfo{Name: node, Arch: runtime.GOARCH},
		Notes:     notes,
		Extra:     map[string]string{},
	}

	client, err := kube.InCluster()
	if err != nil {
		return meta, fmt.Errorf("in cluster config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var obj nodeObject
	if err := client.Get(ctx, "/api/v1/nodes/"+node, &obj); err != nil {
		return meta, fmt.Errorf("node %s: %w", node, err)
	}
	info := obj.Status.NodeInfo
	meta.Node.Name = obj.Metadata.Name
	meta.Node.KernelVersion = info.KernelVersion
	meta.Node.OSImage = info.OSImage
	meta.Node.Arch = info.Architecture
	meta.Node.CPUCores = cpuCores(obj.Status.Capacity["cpu"])
	meta.Node.MemTotalBytes = memBytes(obj.Status.Capacity["memory"])
	meta.Node.CPUModel = cpuModel(obj.Metadata.Labels)
	if meta.Node.CPUModel == "" {
		meta.Node.CPUModel = hostCPUModel()
	}
	if instanceType := obj.Metadata.Labels["node.kubernetes.io/instance-type"]; instanceType != "" {
		meta.Extra["instance_type"] = instanceType
	}
	if fs, err := nodeFilesystem(ctx, client, node); err == nil {
		meta.Node.FSCapacityBytes = fs.Node.Fs.CapacityBytes
		meta.Node.FSAvailableBytes = fs.Node.Fs.AvailableBytes
	}
	meta.Extra["kubelet_version"] = info.KubeletVersion
	meta.Extra["container_runtime"] = info.ContainerRuntimeVersion
	meta.Extra["operating_system"] = info.OperatingSystem

	for _, name := range names {
		engine, err := engines.Get(name)
		if err != nil {
			return meta, err
		}
		meta.Engines = append(meta.Engines, results.EngineMeta{
			Name:     engine.Name,
			Image:    engine.Image,
			Version:  versionOf(engine),
			Args:     engines.CompactArgs(engine.Args),
			CPUMilli: cpuLimitMilli(),
			MemBytes: memLimitBytes(),
			Env:      engine.Env,
			Notes:    engine.Notes,
		})
	}
	return meta, nil
}

func versionOf(e engines.Engine) string {
	if i := strings.LastIndex(e.Image, ":"); i >= 0 {
		return strings.TrimPrefix(e.Image[i+1:], "v")
	}
	return e.Name
}

func cpuCores(value string) int {
	q, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(q)
}

func memBytes(value string) int64 {
	digits := 0
	for digits < len(value) && value[digits] >= '0' && value[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0
	}
	n, err := strconv.ParseInt(value[:digits], 10, 64)
	if err != nil {
		return 0
	}
	switch value[digits:] {
	case "Ki":
		return n * 1024
	case "Mi":
		return n * 1024 * 1024
	case "Gi":
		return n * 1024 * 1024 * 1024
	case "Ti":
		return n * 1024 * 1024 * 1024 * 1024
	}
	return n
}

func cpuModel(labels map[string]string) string {
	for key, value := range labels {
		if strings.Contains(strings.ToLower(key), "cpu-model") {
			return value
		}
	}
	return ""
}

func hostCPUModel() string {
	body, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "model name") {
			if i := strings.Index(line, ":"); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

func memLimitBytes() int64 {
	return benchMemoryLimitBytes
}

func cpuLimitMilli() int64 {
	return benchCPUMilliLimit
}

const (
	benchMemoryLimitBytes = 6 << 30
	benchCPUMilliLimit    = 2000
)

func version() string {
	body, err := os.ReadFile("/tools/version")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(body))
}

func writeMeta(path string, meta results.RunMeta) error {
	if err := results.WriteJSON(path, meta); err != nil {
		return err
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

type nodeFilesystemStats struct {
	Node struct {
		Fs struct {
			CapacityBytes  int64 `json:"capacityBytes"`
			AvailableBytes int64 `json:"availableBytes"`
		} `json:"fs"`
	} `json:"node"`
}

func nodeFilesystem(ctx context.Context, client *kube.Client, node string) (nodeFilesystemStats, error) {
	var out nodeFilesystemStats
	body, err := client.Raw(ctx, "/api/v1/nodes/"+urlEscape(node)+"/proxy/stats/summary")
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}

func urlEscape(value string) string {
	return strings.NewReplacer("/", "%2F", " ", "%20").Replace(value)
}

func main() {
	var (
		node    = flag.String("node", "", "node the engines run on")
		runID   = flag.String("run-id", "", "run id")
		names   = flag.String("engines", "", "comma separated engine names")
		notes   = flag.String("notes", "", "comma separated notes")
		outPath = flag.String("out", "", "path for run.json")
	)
	flag.Parse()
	if *outPath == "" {
		log.Fatal("-out is required")
	}
	m, err := collectMeta(*node, *runID, splitList(*names), splitList(*notes))
	if err != nil {
		log.Fatal(err)
	}
	if err := writeMeta(*outPath, m); err != nil {
		log.Fatal(err)
	}
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
