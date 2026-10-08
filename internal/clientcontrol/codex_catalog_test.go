package clientcontrol

import (
	"encoding/json"
	"testing"
)

func TestCodexCatalogAdvertisesLocalNativeEditProfiles(t *testing.T) {
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(CodexModelCatalog(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 4 {
		t.Fatal("missing local/role aliases")
	}
	for _, model := range catalog.Models {
		if model["apply_patch_tool_type"] != "freeform" || model["shell_type"] != "unified_exec" || model["base_instructions"] == "" {
			t.Fatalf("incomplete native model metadata: %+v", model)
		}
		if model["supports_reasoning_summary_parameter"] != false {
			t.Fatal("private reasoning summary support invented")
		}
	}
}
