package engines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestsMatchRegistry(t *testing.T) {
	for _, e := range All {
		t.Run(e.Name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "..", "deploy", "engines", e.Name, "statefulset.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			manifest := string(body)
			want := []string{
				"image: " + e.Image,
				"- " + e.Binary,
			}
			for _, arg := range e.Args {
				want = append(want, "- "+arg)
			}
			for _, kv := range e.Env {
				name, value, _ := strings.Cut(kv, "=")
				want = append(want, "- name: "+name)
				if !strings.Contains(manifest, "value: "+value) && !strings.Contains(manifest, `value: "`+value+`"`) {
					t.Errorf("env %s=%s missing from manifest", name, value)
				}
			}
			for _, w := range want {
				if !strings.Contains(manifest, w) {
					t.Errorf("manifest lacks %q", w)
				}
			}
			if got := strings.Count(manifest, "            - --"); got != len(e.Args) {
				t.Errorf("manifest has %d engine args, registry has %d", got, len(e.Args))
			}
		})
	}
}

func TestEveryEngineGetsTheSameRuntime(t *testing.T) {
	for _, e := range All {
		if strings.Join(e.Env, " ") != strings.Join(RuntimeEnv, " ") {
			t.Errorf("%s env %v differs from the shared runtime env %v", e.Name, e.Env, RuntimeEnv)
		}
	}
}
