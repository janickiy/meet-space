package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDevelopmentPortsStayOnLoopback checks the committed template without
// loading an operator's .env, contacting Docker, or starting any services.
func TestDevelopmentPortsStayOnLoopback(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Services map[string]struct {
			Ports []string `yaml:"ports"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	for service, ports := range map[string][]string{
		"api": {"APP_HOST_PORT"}, "api-debug": {"APP_HOST_PORT", "DLV_API_PORT"},
		"record-worker": {"WORKER_HOST_PORT"}, "record-worker-debug": {"WORKER_HOST_PORT", "DLV_WORKER_PORT"},
		"postgres": {"POSTGRES_HOST_PORT"}, "redis": {"REDIS_HOST_PORT"},
		"minio":    {"MINIO_PORT_HOST", "MINIO_CONSOLE_PORT_HOST"},
		"rabbitmq": {"RABBIT_MQ_HOST_PORT", "RABBIT_MQ_MANAGEMENT_HOST_PORT"},
	} {
		for _, variable := range ports {
			t.Run(service+"/"+variable, func(t *testing.T) {
				found := false
				for _, port := range config.Services[service].Ports {
					if strings.Contains(port, "${"+variable+":-") {
						found = true
						if !strings.HasPrefix(port, "127.0.0.1:") {
							t.Errorf("%s exposes an internal/development listener beyond loopback", variable)
						}
					}
				}
				if !found {
					t.Fatalf("expected port mapping for %s", variable)
				}
			})
		}
	}
}
