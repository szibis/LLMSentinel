package discovery

import "testing"

func TestEmptyToolSearchPathsReturnUnavailable(t *testing.T) {
	for _, paths := range [][]string{nil, {}, {""}} {
		if got := findTool(paths); got != "" {
			t.Fatalf("empty config selected a tool: %q", got)
		}
	}
}
