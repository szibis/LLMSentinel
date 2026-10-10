package claudemod

import (
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
)

// Explicit files keep development tests and SDK typings out of installed mods.
//
//go:embed plugin/.claude-plugin/plugin.json plugin/hooks/hooks.json plugin/hooks/register.js
var nativeAssets embed.FS

var modOrigin = regexp.MustCompile(`^http://(127\.0\.0\.1|\[::1\])(?::[1-9][0-9]{0,4})?$`)

// Assets builds an installation preview without writing files or resolving the
// executable. The caller owns seeding into its explicitly selected lab plugin.
func Assets(root, endpoint, executable string) (map[string][]byte, error) {
	if err := clientcontrol.ValidateDirectory(root); err != nil {
		return nil, errors.New("mod root must be canonical and absolute without symlinks")
	}
	if _, err := clientcontrol.Assets("claude", endpoint, executable); err != nil {
		return nil, errors.New("invalid mod endpoint or executable")
	}
	// Match the host adapter's accepted literal argument syntax. A slash-suffixed
	// origin or control character would pass generic helpers but fail the host.
	if root == "/" || executable == "/" || !modOrigin.MatchString(endpoint) || strings.IndexFunc(root+executable, func(char rune) bool { return char < 32 || char == 127 }) >= 0 {
		return nil, errors.New("mod configuration contains unsupported argument syntax")
	}
	assets := make(map[string][]byte, 4)
	for _, name := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.js"} {
		data, err := nativeAssets.ReadFile("plugin/" + name)
		if err != nil {
			return nil, errors.New("embedded mod asset unavailable")
		}
		assets[name] = data
	}
	config, err := json.MarshalIndent(map[string]string{"root": root, "endpoint": endpoint, "executable": executable}, "", "  ")
	if err != nil {
		return nil, err
	}
	assets["sentinel-config.json"] = append(config, '\n')
	return assets, nil
}
