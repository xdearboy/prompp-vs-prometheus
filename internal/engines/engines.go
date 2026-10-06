package engines

import "fmt"

type Engine struct {
	Name       string
	Title      string
	Image      string
	Binary     string
	RunAsUser  int64
	RunAsGroup int64
	FSGroup    int64
	Args       []string
	Env        []string
	Notes      string
}

func (e Engine) ArgsWith(extra ...string) []string {
	out := make([]string, 0, len(e.Args)+len(extra))
	out = append(out, e.Args...)
	return append(out, extra...)
}

func commonArgs(configPath, dataPath string) []string {
	return []string{
		"--config.file=" + configPath,
		"--storage.tsdb.path=" + dataPath,
		"--web.enable-remote-write-receiver",
		"--web.enable-lifecycle",
	}
}

var RuntimeEnv = []string{"GOMEMLIMIT=5529MiB", "GOMAXPROCS=2"}

var Prometheus315 = Engine{
	Name:       "prom-3150",
	Title:      "Prometheus 3.15.0",
	Image:      "quay.io/prometheus/prometheus:v3.15.0",
	Binary:     "/bin/prometheus",
	RunAsUser:  65534,
	RunAsGroup: 65534,
	FSGroup:    65534,
	Args:       commonArgs("/etc/prometheus/prometheus.yml", "/prometheus"),
	Env:        RuntimeEnv,
	Notes:      "upstream latest",
}

var Prometheus255 = Engine{
	Name:       "prom-2551",
	Title:      "Prometheus 2.55.1",
	Image:      "quay.io/prometheus/prometheus:v2.55.1",
	Binary:     "/bin/prometheus",
	RunAsUser:  65534,
	RunAsGroup: 65534,
	FSGroup:    65534,
	Args:       commonArgs("/etc/prometheus/prometheus.yml", "/prometheus"),
	Env:        RuntimeEnv,
	Notes:      "closed range selectors, the semantics before 3.0",
}

var Prompp0815 = Engine{
	Name:       "prompp-0815",
	Title:      "Deckhouse Prom++ 0.8.15",
	Image:      "mirror.gcr.io/prompp/prompp:0.8.15",
	Binary:     "/bin/prompp",
	RunAsUser:  64535,
	RunAsGroup: 64535,
	FSGroup:    64535,
	Args:       commonArgs("/etc/prometheus/prometheus.yml", "/prometheus"),
	Env:        RuntimeEnv,
	Notes:      "C++ head and WAL",
}

var All = []Engine{Prompp0815, Prometheus315, Prometheus255}

func Get(name string) (Engine, error) {
	for _, e := range All {
		if e.Name == name {
			return e, nil
		}
	}
	return Engine{}, fmt.Errorf("unknown engine %q, have %v", name, Names())
}

func Names() []string {
	out := make([]string, 0, len(All))
	for _, e := range All {
		out = append(out, e.Name)
	}
	return out
}

func CompactArgs(args []string, extra ...string) []string {
	out := make([]string, 0, len(args)+len(extra))
	for _, a := range args {
		out = append(out, stripValue(a))
	}
	return append(out, extra...)
}

func stripValue(arg string) string {
	for i := 0; i < len(arg); i++ {
		if arg[i] == '=' {
			return arg[:i] + "=..."
		}
	}
	return arg
}
