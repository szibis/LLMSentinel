package cli

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type dashboardTransport func(*http.Request) (*http.Response, error)

func (f dashboardTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func captureDashboard(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	f, e := os.CreateTemp(t.TempDir(), "output")
	if e != nil {
		t.Fatal(e)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old; f.Close() }()
	err := fn()
	f.Seek(0, 0)
	b, _ := io.ReadAll(f)
	return string(b), err
}
func useDashboardTransport(t *testing.T, fn dashboardTransport) {
	t.Helper()
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: fn}
	t.Cleanup(func() { http.DefaultClient = old })
}
func dashboardResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestDashboardViews(t *testing.T) {
	d := NewDashboardCLI("http://dashboard.invalid")
	useDashboardTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/analytics/sentiment-trends":
			return dashboardResponse(200, `{"summary":{"satisfaction_rate":0.9,"total":10,"satisfied":5,"neutral":2,"frustrated":1,"confused":1,"impatient":1},"events":[{"Timestamp":"2026-01-01T12:34:00Z","TaskType":"coding","InitialModel":"haiku","EscalatedTo":"sonnet","Resolved":true},{"Resolved":false}]}`), nil
		case "/api/analytics/budget-status":
			return dashboardResponse(200, `{"daily_budget":{"limit":10,"used":8,"remaining":2,"percentage":80},"monthly_budget":{"limit":100,"used":95,"remaining":5,"days_left":2,"percentage":95},"model_usage":{"haiku":{"Used":8,"Limit":10,"Percentage":80}}}`), nil
		default:
			return dashboardResponse(200, `{"recommendations":[{"task_type":"coding","current_model":"opus","recommended_model":"sonnet","current_satisfaction":0.9,"recommended_satisfaction":0.95,"estimated_savings_percent":25}]}`), nil
		}
	})
	out, err := captureDashboard(t, d.FullDashboard)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"90% ✅ Excellent", "coding (haiku → sonnet)", "$8.00", "80.0% 🟡", "95.0% 🔴", "Average Savings: 25.0%", "$30000.00 saved"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}
func TestDashboardStatusAndEmptyViews(t *testing.T) {
	for _, rate := range []string{"0.7", "0.5"} {
		t.Run(rate, func(t *testing.T) {
			useDashboardTransport(t, func(*http.Request) (*http.Response, error) {
				return dashboardResponse(200, `{"summary":{"satisfaction_rate":`+rate+`},"daily_budget":{"percentage":10},"monthly_budget":{"percentage":80}}`), nil
			})
			d := NewDashboardCLI("http://dashboard.invalid")
			out, err := captureDashboard(t, d.FullDashboard)
			if err != nil {
				t.Fatal(err)
			}
			want := "Good"
			if rate == "0.5" {
				want = "Needs Attention"
			}
			if !strings.Contains(out, want) || !strings.Contains(out, "No optimization opportunities") {
				t.Fatal(out)
			}
		})
	}
}
func TestDashboardErrorsPropagate(t *testing.T) {
	for _, failure := range []string{"transport", "json", "status"} {
		for _, stage := range []string{"sentiment-trends", "budget-status", "cost-optimization"} {
			t.Run(failure+stage, func(t *testing.T) {
				useDashboardTransport(t, func(r *http.Request) (*http.Response, error) {
					if !strings.HasSuffix(r.URL.Path, stage) {
						return dashboardResponse(200, `{}`), nil
					}
					switch failure {
					case "transport":
						return nil, errors.New("offline")
					case "json":
						return dashboardResponse(200, `{`), nil
					default:
						return dashboardResponse(503, `{}`), nil
					}
				})
				d := NewDashboardCLI("http://dashboard.invalid")
				_, err := captureDashboard(t, d.FullDashboard)
				if err == nil {
					t.Fatal("failure was accepted")
				}
			})
		}
	}
}
func TestFetchJSON(t *testing.T) {
	for _, failure := range []string{"ok", "transport", "json", "status"} {
		t.Run(failure, func(t *testing.T) {
			useDashboardTransport(t, func(*http.Request) (*http.Response, error) {
				switch failure {
				case "transport":
					return nil, errors.New("offline")
				case "json":
					return dashboardResponse(200, `{`), nil
				case "status":
					return dashboardResponse(500, "failed"), nil
				default:
					return dashboardResponse(200, `{"value":3}`), nil
				}
			})
			var v map[string]int
			err := fetchJSON("http://dashboard.invalid", &v)
			if failure == "ok" {
				if err != nil || v["value"] != 3 {
					t.Fatal(v, err)
				}
			} else if err == nil {
				t.Fatal("missing error")
			}
		})
	}
}
