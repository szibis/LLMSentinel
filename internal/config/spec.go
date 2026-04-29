package config

import (
	"bytes"
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed CONFIG_SPEC.yaml
var configSpecFS embed.FS

// ConfigSpec represents the complete configuration specification
type ConfigSpec struct {
	Sections map[string]Section `yaml:"sections"`
	Examples map[string]Example `yaml:"examples"`
}

// Section represents a configuration section
type Section struct {
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Icon        string                 `yaml:"icon"`
	Options     map[string]interface{} `yaml:"options"`
}

// Example represents a configuration example
type Example struct {
	Description string `yaml:"description"`
	YAML        string `yaml:"yaml"`
}

// LoadConfigSpec loads the configuration specification from embedded YAML
func LoadConfigSpec() (*ConfigSpec, error) {
	data, err := configSpecFS.ReadFile("CONFIG_SPEC.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config spec: %w", err)
	}

	spec := &ConfigSpec{
		Sections: make(map[string]Section),
		Examples: make(map[string]Example),
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(spec); err != nil {
		return nil, fmt.Errorf("failed to parse config spec: %w", err)
	}

	return spec, nil
}

// GetSectionHint returns formatted HTML hint for a configuration section
func (spec *ConfigSpec) GetSectionHint(sectionName string) string {
	section, exists := spec.Sections[sectionName]
	if !exists {
		return ""
	}

	html := fmt.Sprintf(
		`<strong>%s %s</strong><div style="margin-top: 8px; font-size: 10px; color: #666; line-height: 1.6;">%s</div>`,
		section.Icon,
		section.Title,
		section.Description,
	)

	return html
}

// GetOptionHint returns formatted HTML hint for a configuration option
func (spec *ConfigSpec) GetOptionHint(sectionName, optionName string) string {
	section, exists := spec.Sections[sectionName]
	if !exists {
		return ""
	}

	if option, ok := section.Options[optionName]; ok {
		return formatOptionHint(optionName, option)
	}

	return ""
}

// GetAllOptions returns all options across all sections
func (spec *ConfigSpec) GetAllOptions() map[string]map[string]interface{} {
	allOptions := make(map[string]map[string]interface{})

	for sectionName, section := range spec.Sections {
		allOptions[sectionName] = section.Options
	}

	return allOptions
}

func formatOptionHint(name string, option interface{}) string {
	optionMap, ok := option.(map[string]interface{})
	if !ok {
		return ""
	}

	title := name
	if t, exists := optionMap["title"]; exists {
		title = fmt.Sprintf("%v", t)
	}

	description := ""
	if d, exists := optionMap["description"]; exists {
		description = fmt.Sprintf("%v", d)
	}

	optionType := ""
	if t, exists := optionMap["type"]; exists {
		optionType = fmt.Sprintf("%v", t)
	}

	defaultVal := ""
	if d, exists := optionMap["default"]; exists {
		defaultVal = fmt.Sprintf("%v", d)
	}

	unit := ""
	if u, exists := optionMap["unit"]; exists {
		unit = fmt.Sprintf(" (%v)", u)
	}

	html := fmt.Sprintf(
		`<strong>%s</strong><div style="margin-top: 8px; font-size: 10px; color: #666; line-height: 1.6;">`,
		title,
	)

	if description != "" {
		html += fmt.Sprintf(`%s<br>`, description)
	}

	if optionType != "" {
		html += fmt.Sprintf(`<br><strong>Type:</strong> %s%s<br>`, optionType, unit)
	}

	if defaultVal != "" {
		html += fmt.Sprintf(`<strong>Default:</strong> %s<br>`, defaultVal)
	}

	html += `</div>`

	return html
}
