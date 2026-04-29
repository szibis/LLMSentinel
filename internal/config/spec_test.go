package config

import (
	"testing"
)

func TestLoadConfigSpec_ReturnsValidSpec(t *testing.T) {
	spec, err := LoadConfigSpec()
	if err != nil {
		t.Errorf("LoadConfigSpec returned error: %v", err)
	}
	if spec == nil {
		t.Error("LoadConfigSpec returned nil spec")
	}
}

func TestLoadConfigSpec_ContainsRequiredSections(t *testing.T) {
	spec, _ := LoadConfigSpec()
	expectedSections := []string{
		"gateway",
		"optimizations",
		"security",
		"metrics",
	}
	for _, section := range expectedSections {
		if _, ok := spec.Sections[section]; !ok {
			t.Errorf("Config spec missing section: %s", section)
		}
	}
}

func TestLoadConfigSpec_SectionsHaveTitle(t *testing.T) {
	spec, _ := LoadConfigSpec()
	for name, section := range spec.Sections {
		if section.Title == "" {
			t.Errorf("Section %s missing title", name)
		}
	}
}

func TestLoadConfigSpec_SectionsHaveDescription(t *testing.T) {
	spec, _ := LoadConfigSpec()
	for name, section := range spec.Sections {
		if section.Description == "" {
			t.Errorf("Section %s missing description", name)
		}
	}
}

func TestGetSectionHint_ReturnsForKnownSection(t *testing.T) {
	spec, _ := LoadConfigSpec()
	hint := spec.GetSectionHint("gateway")
	if hint == "" {
		t.Error("GetSectionHint returned empty hint for known section")
	}
}

func TestGetAllOptions_ReturnsDictionary(t *testing.T) {
	spec, _ := LoadConfigSpec()
	allOptions := spec.GetAllOptions()
	if allOptions == nil {
		t.Error("GetAllOptions returned nil")
	}
	if len(allOptions) == 0 {
		t.Error("GetAllOptions returned empty map")
	}
}

func TestLoadConfigSpec_ContainsExamples(t *testing.T) {
	spec, _ := LoadConfigSpec()
	if spec.Examples == nil {
		t.Error("Config spec has nil examples")
	}
}
