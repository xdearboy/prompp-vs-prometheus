package report

import (
	"strings"
	"testing"
)

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{[]float64{3}, 3},
		{[]float64{1, 5, 9}, 5},
		{[]float64{1, 2, 8, 9}, 5},
	}
	for _, c := range cases {
		if got := median(c.in); got != c.want {
			t.Errorf("median(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAggregatePublishedRun(t *testing.T) {
	body, err := Aggregate("../../runs/series-20261006", []string{"20261006T210616Z"}, "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Prom++ 0.8.15", "Prometheus 2.55.1", "Query geomean p50, concurrency 16"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("aggregate lacks %q", want)
		}
	}
}
