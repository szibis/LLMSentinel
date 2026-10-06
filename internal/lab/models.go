package lab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type runtimeSettings struct {
	ModelPath          string `json:"model_path"`
	SmallModelPath     string `json:"small_model_path,omitempty"`
	Executable         string `json:"executable"`
	ModelRevision      string `json:"model_revision,omitempty"`
	SmallModelRevision string `json:"small_model_revision,omitempty"`
}
type runtimeItem struct {
	Name      string   `json:"name"`
	Port      int      `json:"port"`
	ModelPath string   `json:"model_path"`
	Command   []string `json:"-"`
}

func nonemptyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("missing or empty file: %s", filepath.Base(path))
	}
	return nil
}
func validateModel(path string, role bool) error {
	if !filepath.IsAbs(path) {
		return errors.New("MODEL_PATH must be an absolute cached directory, not a Hub ID")
	}
	for _, name := range []string{"config.json", "tokenizer.json"} {
		if err := nonemptyFile(filepath.Join(path, name)); err != nil {
			return err
		}
	}
	weights, err := filepath.Glob(filepath.Join(path, "*.safetensors"))
	if err != nil || len(weights) == 0 {
		return errors.New("missing model weights")
	}
	for _, weight := range weights {
		if err := nonemptyFile(weight); err != nil {
			return err
		}
	}
	indexPath := filepath.Join(path, "model.safetensors.index.json")
	var index struct {
		Weights map[string]string `json:"weight_map"`
	}
	if data, err := os.ReadFile(indexPath); err == nil {
		if json.Unmarshal(data, &index) != nil || len(index.Weights) == 0 {
			return errors.New("model index has no valid weight_map")
		}
		for _, shard := range index.Weights {
			if filepath.Base(shard) != shard || nonemptyFile(filepath.Join(path, shard)) != nil {
				return errors.New("model has missing or unsafe weight shard")
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		for _, weight := range weights {
			if strings.Contains(filepath.Base(weight), "-of-") {
				return errors.New("sharded model needs model.safetensors.index.json")
			}
		}
	}
	if role {
		var architecture struct {
			ModelType string `json:"model_type"`
		}
		if readJSON(filepath.Join(path, "config.json"), &architecture) != nil {
			return errors.New("invalid config.json")
		}
		var config struct {
			Template string `json:"chat_template"`
		}
		if err := nonemptyFile(filepath.Join(path, "tokenizer_config.json")); err != nil {
			return err
		}
		if err := readJSON(filepath.Join(path, "tokenizer_config.json"), &config); err != nil {
			return errors.New("invalid tokenizer_config.json")
		}
		template := config.Template
		if data, err := os.ReadFile(filepath.Join(path, "chat_template.jinja")); err == nil {
			template = string(data)
		}
		switch architecture.ModelType {
		case "lfm2_moe":
			if !strings.Contains(template, "<think>") || !strings.Contains(template, "</think>") {
				return errors.New("LFM role model needs its reasoning chat template")
			}
		case "gemma4":
			if !strings.Contains(template, "enable_thinking") || !strings.Contains(template, "<|channel>thought") || !strings.Contains(template, "<channel|>") {
				return errors.New("gemma role model needs its thinking channel chat template")
			}
		case "qwen3", "qwen3_moe", "qwen3_5", "qwen3_5_moe", "qwen3_next":
			if !strings.Contains(template, "enable_thinking") {
				return errors.New("qwen role model needs its thinking chat template")
			}
		default:
			return fmt.Errorf("unsupported role model_type %q", architecture.ModelType)
		}
	}
	return nil
}
func (e *environment) runtimePlan(settings runtimeSettings) ([]runtimeItem, error) {
	if settings.ModelPath == "" {
		return nil, nil
	}
	models := []runtimeItem{{Name: "large", Port: 19091, ModelPath: settings.ModelPath}}
	if settings.SmallModelPath != "" {
		models = append(models, runtimeItem{Name: "small", Port: 19092, ModelPath: settings.SmallModelPath})
	}
	for _, model := range models {
		if err := validateModel(model.ModelPath, len(models) == 2); err != nil {
			return nil, err
		}
	}
	executable, err := exec.LookPath(settings.Executable)
	if err != nil {
		return nil, errors.New("MLX-Flash not installed; set MLX_FLASH_BIN to an existing executable; no installation attempted")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	for i := range models {
		models[i].Command = []string{executable, "--model", models[i].ModelPath, "--host", "127.0.0.1", "--port", fmt.Sprint(models[i].Port), "--speculative", "none"}
	}
	return models, nil
}
func (e *environment) gatewayCommand(settings runtimeSettings) []string {
	args := []string{filepath.Join(e.project, "bin", "sentinel-gateway")}
	if settings.SmallModelPath != "" {
		args = append(args, "--role-haiku-upstream", "http://127.0.0.1:19092/v1", "--role-sonnet-upstream", "http://127.0.0.1:19091/v1", "--role-opus-upstream", "http://127.0.0.1:19091/v1")
	}
	return args
}
