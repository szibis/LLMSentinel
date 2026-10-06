package lab

import (
	"path/filepath"
	"slices"
	"strings"
)

// legacyWords only recognizes literal shell words, never expansions or operators.
func legacyWords(command string) []string {
	var words []string
	var word strings.Builder
	var quoted rune
	active := false
	for _, char := range command {
		if quoted != 0 {
			if char == quoted {
				quoted = 0
			} else {
				word.WriteRune(char)
			}
			continue
		}
		switch char {
		case '\'', '"':
			quoted = char
			active = true
		case ' ', '\t':
			if active {
				words = append(words, word.String())
				word.Reset()
				active = false
			}
		case '$', '`', '\\', ';', '|', '&', '<', '>', '\n', '\r':
			return nil
		default:
			word.WriteRune(char)
			active = true
		}
	}
	if quoted != 0 {
		return nil
	}
	if active {
		words = append(words, word.String())
	}
	return words
}

func legacyPython(words []string) []string {
	if len(words) > 0 && words[0] == "rtk" {
		words = words[1:]
	}
	if len(words) == 0 {
		return nil
	}
	binary := filepath.Base(words[0])
	if binary != "python" && binary != "python3" && !strings.HasPrefix(binary, "python3.") {
		return nil
	}
	return words[1:]
}
func (e *environment) ownedLegacyStatus(command string) bool {
	words := legacyPython(legacyWords(command))
	return slices.Equal(words, []string{filepath.Join(e.project, "scripts", "sentinel_statusline.py"), "--root", e.root})
}
func (e *environment) ownedLegacyAsset(old, current string) bool {
	before, after, ok := strings.Cut(current, "```sh\n")
	if !ok {
		return false
	}
	expectedPrefix := strings.Replace(before, "the control command itself", "the Python command itself", 1)
	oldBefore, oldAfter, ok := strings.Cut(old, "```sh\n")
	if !ok || oldBefore != expectedPrefix {
		return false
	}
	command, suffix, ok := strings.Cut(after, "\n```\n")
	if !ok || suffix != "" {
		return false
	}
	oldCommand, oldSuffix, ok := strings.Cut(oldAfter, "\n```\n")
	if !ok || oldSuffix != "" {
		return false
	}
	currentWords := legacyWords(command)
	if len(currentWords) < 4 {
		return false
	}
	expected := append([]string{filepath.Join(e.project, "scripts", "sentinel_control.py")}, currentWords[2:]...)
	return slices.Equal(legacyPython(legacyWords(oldCommand)), expected)
}
