package plugins

import (
	"context"
	"testing"
)

func TestRTKPassthroughDoesNotInventSavings(t *testing.T) {
	p := NewRTKOptimizationPlugin()
	if p.Name() == "" || p.Description() == "" || p.Version() == "" {
		t.Fatal("missing identity")
	}
	if err := p.Initialize(map[string]interface{}{"enabled": false, "saving_rate": 50.0}); err != nil {
		t.Fatal(err)
	}
	input := map[string]string{"command": "echo"}
	out, m, err := p.OptimizeInput(context.Background(), input)
	if err != nil || m.TokensSaved != 0 || out.(map[string]string)["command"] != "echo" {
		t.Fatal(out, m, err)
	}
	for _, value := range []interface{}{"", "actual output", nil} {
		out, m, err := p.OptimizeOutput(context.Background(), value)
		if err != nil || out != value || m.TokensSaved != 0 || m.TokensOut != 0 || m.SavingsPercent != 0 {
			t.Errorf("passthrough invented accounting: output=%v metrics=%+v err=%v", out, m, err)
		}
	}
	if p.GetMetrics().TotalTokensSaved != 0 || p.GetMetrics().AverageSavingsPercent != 0 {
		t.Fatal("invented aggregate savings")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
