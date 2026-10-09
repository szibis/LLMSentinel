// Package testcoverage enforces measured Go statement coverage without hiding
// packages outside an explicitly selected component scope.
package testcoverage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Package struct {
	Name    string  `json:"name"`
	Total   int64   `json:"statements"`
	Covered int64   `json:"covered"`
	Percent float64 `json:"percent"`
}
type Report struct {
	Total    int64     `json:"statements"`
	Covered  int64     `json:"covered"`
	Percent  float64   `json:"percent"`
	Minimum  float64   `json:"minimum"`
	Scope    []string  `json:"scope"`
	Packages []Package `json:"packages"`
	Passed   bool      `json:"passed"`
}
type block struct {
	statements int64
	covered    bool
}

var linePattern = regexp.MustCompile(`^(.+):([0-9]+\.[0-9]+,[0-9]+\.[0-9]+) ([0-9]+) ([0-9]+)$`)

func Analyze(reader io.Reader, minimum float64, scope []string) (Report, error) {
	report := Report{Minimum: minimum, Scope: append([]string{}, scope...), Packages: []Package{}}
	if reader == nil || math.IsNaN(minimum) || math.IsInf(minimum, 0) || minimum <= 0 || minimum > 100 {
		return report, errors.New("coverage requires a profile and minimum in (0,100]")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, 32*1024*1024+1))
	if err != nil {
		return report, err
	}
	if len(raw) > 32*1024*1024 || !utf8.Valid(raw) {
		return report, errors.New("invalid/oversized coverage profile")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 256*1024)
	if !scanner.Scan() {
		return report, errors.New("missing coverage mode")
	}
	switch scanner.Text() {
	case "mode: set", "mode: count", "mode: atomic":
	default:
		return report, errors.New("invalid coverage mode")
	}
	blocks := map[string]block{}
	packages := map[string]*Package{}
	for scanner.Scan() {
		if scanner.Text() == "" {
			continue
		}
		parts := linePattern.FindStringSubmatch(scanner.Text())
		if parts == nil {
			return report, errors.New("invalid coverage block")
		}
		statements, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || statements > 1000000000 {
			return report, errors.New("invalid coverage statement count")
		}
		count, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return report, errors.New("invalid coverage execution count")
		}
		key := parts[1] + ":" + parts[2]
		old, present := blocks[key]
		if present {
			if old.statements != statements {
				return report, errors.New("conflicting coverage blocks")
			}
			old.covered = old.covered || count > 0
			blocks[key] = old
		} else {
			blocks[key] = block{statements: statements, covered: count > 0}
		}
		name := path.Dir(parts[1])
		if packages[name] == nil {
			packages[name] = &Package{Name: name}
		}
	}
	if err := scanner.Err(); err != nil {
		return report, err
	}
	for key, b := range blocks {
		name := path.Dir(key[:strings.LastIndex(key, ":")])
		p := packages[name]
		if report.Total > 1000000000-b.statements {
			return report, errors.New("coverage total exceeds bound")
		}
		report.Total += b.statements
		p.Total += b.statements
		if b.covered {
			report.Covered += b.statements
			p.Covered += b.statements
		}
	}
	if report.Total == 0 {
		return report, errors.New("coverage contains no statements")
	}
	report.Percent = 100 * float64(report.Covered) / float64(report.Total)
	for _, p := range packages {
		if p.Total > 0 {
			p.Percent = 100 * float64(p.Covered) / float64(p.Total)
		}
		report.Packages = append(report.Packages, *p)
	}
	sort.Slice(report.Packages, func(i, j int) bool { return report.Packages[i].Name < report.Packages[j].Name })
	report.Passed = report.Percent >= minimum
	if len(scope) > 0 {
		report.Passed = true
		for _, name := range scope {
			p := packages[name]
			if p == nil || p.Total == 0 {
				return report, fmt.Errorf("coverage scope missing statements: %s", name)
			}
			report.Passed = report.Passed && p.Percent >= minimum
		}
	}
	return report, nil
}

type scopes []string

func (s *scopes) String() string { return strings.Join(*s, ",") }
func (s *scopes) Set(value string) error {
	if value == "" {
		return errors.New("empty coverage package")
	}
	*s = append(*s, value)
	return nil
}

func Run(args []string, _ io.Reader, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("coverage", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	profile := flags.String("profile", "coverage.out", "Go coverage profile")
	minimum := flags.Float64("min", 90, "minimum unrounded statement percentage")
	var scope scopes
	flags.Var(&scope, "package", "exact package to enforce (repeatable); complete repository report remains included")
	if flags.Parse(args) != nil {
		return 2
	}
	if flags.NArg() != 0 || math.IsNaN(*minimum) || math.IsInf(*minimum, 0) || *minimum <= 0 || *minimum > 100 {
		fmt.Fprintln(diagnostic, "invalid coverage arguments")
		return 2
	}
	file, err := os.Open(*profile)
	if err != nil {
		fmt.Fprintln(diagnostic, err)
		return 1
	}
	defer file.Close()
	report, err := Analyze(file, *minimum, scope)
	if err != nil {
		fmt.Fprintln(diagnostic, err)
		return 1
	}
	if err := json.NewEncoder(out).Encode(report); err != nil {
		fmt.Fprintln(diagnostic, err)
		return 1
	}
	if !report.Passed {
		fmt.Fprintln(diagnostic, "statement coverage below required minimum")
		return 1
	}
	return 0
}
