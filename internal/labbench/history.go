package labbench

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxHistory = 20
const historyDirectory = "benchmark-history"

func archiveName(r Report) (string, error) {
	id, err := hex.DecodeString(r.RunID)
	if err != nil || len(id) != 16 || r.RunID != hex.EncodeToString(id) || r.Timestamp.IsZero() {
		return "", errors.New("benchmark history requires a timestamp and lowercase random run ID")
	}
	return r.Timestamp.UTC().Format("20060102T150405.000000000Z") + "-" + r.RunID + ".json", nil
}

func historyFiles(root string) (string, []os.DirEntry, error) {
	dir := filepath.Join(root, historyDirectory)
	stat, err := os.Lstat(dir)
	if err != nil || !stat.IsDir() {
		return dir, nil, errors.New("benchmark history directory unavailable or unsafe")
	}
	f, err := os.Open(dir)
	if err != nil {
		return dir, nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(maxHistory + 2)
	if err != nil && !errors.Is(err, io.EOF) {
		return dir, nil, err
	}
	if len(entries) > maxHistory+1 {
		return dir, nil, errors.New("benchmark history exceeds retention bound")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if !entry.Type().IsRegular() || len(entry.Name()) != len("20060102T150405.000000000Z-")+32+len(".json") {
			return dir, nil, errors.New("benchmark history contains unsupported entries")
		}
		r, err := loadReport(filepath.Join(dir, entry.Name()))
		if err != nil {
			return dir, nil, err
		}
		name, err := archiveName(r)
		if err != nil || name != entry.Name() {
			return dir, nil, errors.New("benchmark history identity mismatch")
		}
	}
	return dir, entries, nil
}

func archiveReport(root string, r Report, raw []byte) error {
	if len(raw) > maxBody {
		return errors.New("benchmark archive exceeds report bound")
	}
	name, err := archiveName(r)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, historyDirectory)
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	_, existing, err := historyFiles(root)
	if err != nil {
		return err
	}
	for _, entry := range existing {
		if strings.HasSuffix(entry.Name(), "-"+r.RunID+".json") && entry.Name() != name {
			return errors.New("benchmark run identity already has different evidence")
		}
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		previous, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(previous, raw) {
			return nil
		}
		return errors.New("benchmark run identity already has different evidence")
	}
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		f.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, entries, err := historyFiles(root)
	if err != nil {
		return err
	}
	for len(entries) > maxHistory {
		if err := os.Remove(filepath.Join(dir, entries[0].Name())); err != nil {
			return err
		}
		entries = entries[1:]
	}
	complete = true
	return nil
}

// LoadHistory returns separately identified runs, newest first. No measurements
// are combined across runs, models, or runtime counter windows.
func LoadHistory(root string) ([]Report, error) {
	dir, entries, err := historyFiles(root)
	if err != nil {
		return nil, err
	}
	reports := make([]Report, 0, maxHistory)
	for i := len(entries) - 1; i >= 0 && len(reports) < maxHistory; i-- {
		r, err := loadReport(filepath.Join(dir, entries[i].Name()))
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}
