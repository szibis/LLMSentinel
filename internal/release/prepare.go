package release

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type prLabel struct {
	Name string `json:"name"`
}
type prBase struct {
	Ref string `json:"ref"`
}
type mergedPR struct {
	Title    string    `json:"title"`
	MergeSHA string    `json:"merge_commit_sha"`
	MergedAt string    `json:"merged_at"`
	Base     prBase    `json:"base"`
	Labels   []prLabel `json:"labels"`
}
type releasePlan struct {
	Mode    string `json:"mode"`
	Bump    string `json:"bump,omitempty"`
	Version string `json:"version,omitempty"`
}

var conventional = regexp.MustCompile(`^(feat|fix|perf|refactor)(\([^\r\n]+\))?(!)?:\s+\S`)
var releaseTitle = regexp.MustCompile(`^chore: release (v[0-9]+\.[0-9]+\.[0-9]+)$`)

func selectPlan(prs []mergedPR, sha string) (releasePlan, error) {
	plan := releasePlan{Mode: "skip"}
	if !commitPattern.MatchString(sha) {
		return plan, errors.New("invalid tested commit SHA")
	}
	var selected *mergedPR
	for i := range prs {
		pr := &prs[i]
		if pr.MergedAt != "" && pr.Base.Ref == "main" && pr.MergeSHA == sha {
			if selected != nil {
				return plan, errors.New("ambiguous merged PRs for tested commit")
			}
			selected = pr
		}
	}
	if selected == nil {
		return plan, nil
	}
	bump := ""
	release := false
	for _, label := range selected.Labels {
		name := label.Name
		if name == "release" {
			release = true
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), "release:") {
			value := strings.TrimPrefix(name, "release:")
			if (value != "major" && value != "minor" && value != "patch") || bump != "" {
				return plan, errors.New("unknown or ambiguous release bump labels")
			}
			bump = value
		}
	}
	title := releaseTitle.FindStringSubmatch(selected.Title)
	if release || strings.HasPrefix(selected.Title, "chore: release ") {
		if !release || len(title) != 2 || bump != "" {
			return plan, errors.New("release PR requires release label, strict title, and no bump label")
		}
		if _, err := versionKey(title[1]); err != nil {
			return plan, err
		}
		return releasePlan{Mode: "publish", Version: title[1]}, nil
	}
	if bump == "" {
		match := conventional.FindStringSubmatch(strings.ToLower(selected.Title))
		if len(match) == 0 {
			return plan, nil
		}
		switch {
		case match[3] == "!":
			bump = "major"
		case match[1] == "feat":
			bump = "minor"
		default:
			bump = "patch"
		}
	}
	return releasePlan{Mode: "prepare", Bump: bump}, nil
}

func nextVersion(latest, bump string) (string, error) {
	key, err := versionKey(latest)
	if err != nil {
		return "", err
	}
	index := 2
	switch bump {
	case "major":
		index = 0
	case "minor":
		index = 1
	case "patch":
	default:
		return "", errors.New("bump must be major, minor, or patch")
	}
	if key[index] == math.MaxUint64 {
		return "", errors.New("version overflow")
	}
	key[index]++
	for i := index + 1; i < 3; i++ {
		key[i] = 0
	}
	return fmt.Sprintf("v%d.%d.%d", key[0], key[1], key[2]), nil
}

func prepare(root, latest, bump, date, notes string) (string, error) {
	return prepareWithFloor(root, latest, "", bump, date, notes)
}

func prepareWithFloor(root, latest, floor, bump, date, notes string) (string, error) {
	base := latest
	if floor != "" {
		published, err := versionKey(latest)
		if err != nil {
			return "", err
		}
		reviewed, err := versionKey(floor)
		if err != nil {
			return "", err
		}
		for i := range reviewed {
			if reviewed[i] > published[i] {
				base = floor
				break
			}
			if reviewed[i] < published[i] {
				break
			}
		}
	}
	version, err := nextVersion(base, bump)
	if err != nil {
		return "", err
	}
	if _, err = time.Parse("2006-01-02", date); err != nil {
		return "", errors.New("date must be valid YYYY-MM-DD")
	}
	path := filepath.Join(root, "CHANGELOG.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	} // #nosec G703 -- Fixed filename under explicitly selected local repository root.
	text := string(content)
	heading := "## [" + version[1:] + "] - "
	if strings.Contains(text, heading) {
		current, e := os.ReadFile(filepath.Join(root, "VERSION")) // #nosec G703 -- Fixed release metadata filename under local repository root.
		if e != nil || strings.TrimSpace(string(current)) != version || strings.Count(text, heading) != 1 {
			return "", errors.New("existing changelog section does not match VERSION")
		}
		return version, nil
	}
	// Keep the preamble and all historical sections byte for byte. Move Unreleased
	// notes into the new dated section, leaving an empty Unreleased for future work.
	insertion := strings.Index(text, "\n## [")
	if insertion < 0 {
		insertion = len(text)
	} else {
		insertion++
	}
	prefix, suffix := text[:insertion], text[insertion:]
	unreleased := ""
	if strings.HasPrefix(suffix, "## [Unreleased]") {
		end := strings.Index(suffix, "\n## [")
		if end < 0 {
			end = len(suffix)
		} else {
			end++
		}
		headerEnd := strings.IndexByte(suffix, '\n')
		if headerEnd < 0 {
			headerEnd = end
		}
		unreleased = strings.TrimSpace(suffix[headerEnd:end])
		prefix += "## [Unreleased]\n\n"
		suffix = suffix[end:]
	}
	var subjects []string
	seen := map[string]bool{}
	for _, line := range strings.Split(unreleased, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			seen[strings.TrimPrefix(line, "- ")] = true
		}
	}
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			subjects = append(subjects, "- "+line)
		}
	}
	if len(subjects) == 0 && unreleased == "" {
		return "", errors.New("release notes must not be empty")
	}
	body := heading + date + "\n\n"
	if unreleased != "" {
		body += unreleased + "\n\n"
	}
	if len(subjects) > 0 {
		body += "### Commit subjects since " + latest + "\n\n" + strings.Join(subjects, "\n") + "\n\n"
	}
	text = strings.TrimRight(prefix, "\n") + "\n\n" + body + suffix
	if err = os.WriteFile(path, []byte(text), 0600); err != nil {
		return "", err
	} // #nosec G703 -- Fixed metadata filename under explicitly selected local repository root.
	if err = os.WriteFile(filepath.Join(root, "VERSION"), []byte(version+"\n"), 0600); err != nil {
		return "", err
	} // #nosec G703 -- Fixed metadata filename under explicitly selected local repository root.
	return version, nil
}

func runPrepare(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("release prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	latest := flags.String("latest", "", "latest stable tag")
	floor := flags.String("version-floor", "", "reviewed repository VERSION; next version exceeds both this and latest tag")
	bump := flags.String("bump", "", "major, minor, or patch")
	date := flags.String("date", "", "release date YYYY-MM-DD")
	notes := flags.String("notes", "", "file of git commit subjects")
	planMode := flags.Bool("plan", false, "select merged PR release action as JSON")
	prs := flags.String("prs", "", "GitHub associated PR JSON file")
	sha := flags.String("sha", "", "tested main SHA")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected arguments")
		return 2
	}
	var err error
	if *planMode {
		var data []byte
		data, err = os.ReadFile(*prs) // #nosec G703 -- Explicit local CLI input file.
		if err == nil {
			var values []mergedPR
			err = json.Unmarshal(data, &values)
			if err == nil {
				var plan releasePlan
				plan, err = selectPlan(values, *sha)
				if err == nil {
					err = json.NewEncoder(out).Encode(plan)
				}
			}
		}
	} else {
		var data []byte
		data, err = os.ReadFile(*notes) // #nosec G703 -- Explicit local CLI input file.
		if err == nil {
			var version string
			version, err = prepareWithFloor(*root, *latest, *floor, *bump, *date, string(data))
			if err == nil {
				_, err = fmt.Fprintln(out, version)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
