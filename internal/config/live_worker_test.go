package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLiveWorkerIsOptional checks committed Compose files without starting Docker.
func TestLiveWorkerIsOptional(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "../..")
	for _, filename := range []string{"docker-compose.yml", "docker-compose.production.yml"} {
		t.Run(filename, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filename))
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Services map[string]struct {
					Profiles  []string `yaml:"profiles"`
					DependsOn any      `yaml:"depends_on"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			profiles := config.Services["live-worker"].Profiles
			if len(profiles) != 1 || profiles[0] != "live" {
				t.Fatalf("live-worker must require explicit live profile, got %v", profiles)
			}
			for name, service := range config.Services {
				switch dependencies := service.DependsOn.(type) {
				case map[string]any:
					if _, required := dependencies["live-worker"]; required {
						t.Errorf("%s requires an optional live-worker", name)
					}
				case []any:
					for _, dependency := range dependencies {
						if dependency == "live-worker" {
							t.Errorf("%s requires an optional live-worker", name)
						}
					}
				}
			}
		})
	}
}

// TestOptionalLiveMonitoring has no disabled target that could trigger down alerts.
func TestOptionalLiveMonitoring(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "../../dockers/observability")
	for _, fixture := range []struct {
		filename string
		count    int
	}{{"live-targets.disabled.json", 0}, {"live-targets.enabled.json", 1}} {
		data, err := os.ReadFile(filepath.Join(root, fixture.filename))
		if err != nil {
			t.Fatal(err)
		}
		var targets []struct {
			Targets []string `json:"targets"`
		}
		if err := json.Unmarshal(data, &targets); err != nil || len(targets) != fixture.count {
			t.Fatalf("%s: target count=%d error=%v", fixture.filename, len(targets), err)
		}
		if fixture.count == 1 && (len(targets[0].Targets) != 1 || targets[0].Targets[0] != "live-worker:8093") {
			t.Fatal("enabled metrics must target the live-worker's private port")
		}
	}
}
