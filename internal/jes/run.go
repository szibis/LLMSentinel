package jes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/taskquality"
)

type Request struct {
	Version int      `json:"version"`
	Samples []Sample `json:"samples"`
	Policy  Policy   `json:"policy"`
}
type Report struct {
	Version          int       `json:"version"`
	Scope            string    `json:"scope"`
	FinishedAt       time.Time `json:"finished_at"`
	Outcomes         []Outcome `json:"outcomes"`
	TrainingEligible int       `json:"training_eligible"`
}

const reportName = "jes-quality-latest.json"

func decodeRequest(raw []byte) (Request, error) {
	var request Request
	if err := taskquality.DecodeEvidenceJSON(raw, &request); err != nil {
		return request, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	if request.Version != 1 || len(request.Samples) == 0 || len(request.Samples) > 256 {
		return request, errors.New("unsupported or empty quality request")
	}
	seen := map[string]bool{}
	for _, sample := range request.Samples {
		if sample.ID == "" || seen[sample.ID] {
			return request, errors.New("sample IDs must be unique and nonempty")
		}
		seen[sample.ID] = true
	}
	return request, nil
}

func writeReport(path string, raw []byte) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err = clientcontrol.ValidateDirectory(filepath.Dir(absolute)); err != nil {
		return err
	}
	if stat, err := os.Lstat(absolute); err == nil && !stat.Mode().IsRegular() {
		return errors.New("report target must be a regular file")
	}
	file, err := os.CreateTemp(filepath.Dir(absolute), ".jes-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, absolute)
}

func Run(args []string, in io.Reader, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("jes", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	output := flags.String("output", "", "optional private quality report path")
	taskReport := flags.String("task-report", "", "optional bounded synthetic task-quality summary; training admission always disabled")
	source := flags.String("source-revision", "", "actual source revision, unknown when omitted")
	model := flags.String("model-revision", "", "actual model revision, unknown when omitted")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	if *taskReport != "" {
		file, err := os.Open(*taskReport)
		if err != nil {
			fmt.Fprintln(diagnostic, "task evidence unavailable")
			return 1
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			fmt.Fprintln(diagnostic, "task summary unavailable or oversized")
			return 1
		}
		request, err := requestFromTaskReport(data, *source, *model)
		if err != nil {
			fmt.Fprintln(diagnostic, "task summary invalid")
			return 1
		}
		data, err = json.Marshal(request)
		if err != nil {
			fmt.Fprintln(diagnostic, "task evidence unavailable")
			return 1
		}
		in = bytes.NewReader(data)
	}
	if in == nil {
		fmt.Fprintln(diagnostic, "quality input required")
		return 1
	}
	raw, err := io.ReadAll(io.LimitReader(in, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		fmt.Fprintln(diagnostic, "quality input unavailable or oversized")
		return 1
	}
	request, err := decodeRequest(raw)
	if err != nil {
		fmt.Fprintln(diagnostic, "invalid quality request")
		return 1
	}
	report := Report{Version: 1, Scope: "advisory-fixture-quality", FinishedAt: time.Now().UTC(), Outcomes: []Outcome{}}
	for _, sample := range request.Samples {
		outcome, err := Evaluate(context.Background(), EvidenceJudge{}, sample, request.Policy)
		if err != nil {
			fmt.Fprintln(diagnostic, err)
			return 1
		}
		report.Outcomes = append(report.Outcomes, outcome)
		if outcome.TrainingEligible {
			report.TrainingEligible++
		}
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(diagnostic, "quality report unavailable")
		return 1
	}
	if *output != "" {
		if err = writeReport(*output, raw); err != nil {
			fmt.Fprintln(diagnostic, "cannot persist quality report")
			return 1
		}
	}
	if _, err = out.Write(append(raw, '\n')); err != nil {
		return 1
	}
	return 0
}

func requestFromTaskReport(raw []byte, source, model string) (Request, error) {
	var report taskquality.Report
	if taskquality.DecodeEvidenceJSON(raw, &report) != nil || report.Version != 1 || report.FixtureVersion != 2 || (report.Scope != "real-cli-task-probes" && report.Scope != "bounded-api-task-probes") || len(report.Results) == 0 || len(report.Results) > 96 || report.FinishedAt.IsZero() {
		return Request{}, errors.New("unsupported task evidence")
	}
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	request := Request{Version: 1, Policy: Policy{MinimumScore: 1}, Samples: []Sample{}}
	for _, result := range report.Results {
		_, finalMeasured := result.Checks["final_assertion"]
		_, readMeasured := result.Checks["fixture_read"]
		_, protocolMeasured := result.Checks["protocol_valid"]
		complete := finalMeasured && readMeasured && protocolMeasured
		if result.Task == "coding-fix" && result.Client != "" {
			complete = complete && result.Checks["independent_tests"] && result.Checks["edit_and_tests_preserved"]
		}
		request.Samples = append(request.Samples, Sample{ID: result.Task + "/" + result.Role + "/" + result.Protocol, CorpusVersion: report.FixtureVersion, CorpusSHA256: report.CorpusSHA256, SourceRevision: source, ModelRevision: model, EvidenceSHA256: digest, Model: "", Role: result.Role, ProtocolValid: result.Checks["protocol_valid"], Grounded: result.Checks["fixture_read"], AssertionPassed: result.Checks["final_assertion"], ChecksComplete: complete, Synthetic: true})
	}
	return request, nil
}

func Load(root string) *Report {
	path := filepath.Join(root, reportName)
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var report Report
	if taskquality.DecodeEvidenceJSON(raw, &report) != nil || report.Version != 1 || report.Scope != "advisory-fixture-quality" || report.FinishedAt.IsZero() || len(report.Outcomes) == 0 || len(report.Outcomes) > 256 {
		return nil
	}
	eligible := 0
	seen := map[string]bool{}
	for _, outcome := range report.Outcomes {
		if outcome.Sample == nil || outcome.Policy == nil || outcome.SampleID == "" || seen[outcome.SampleID] {
			return nil
		}
		seen[outcome.SampleID] = true
		expected, err := Evaluate(context.Background(), EvidenceJudge{}, *outcome.Sample, *outcome.Policy)
		if err != nil || !reflect.DeepEqual(outcome, expected) {
			return nil
		}
		if outcome.TrainingEligible {
			eligible++
		}
	}
	if eligible != report.TrainingEligible {
		return nil
	}
	return &report
}
