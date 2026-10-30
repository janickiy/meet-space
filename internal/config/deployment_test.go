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

// TestRecorderDoesNotExposeWebRTCPorts ensures only the SFU accepts media
// connections; the recorder keeps its loopback administration and debug ports.
func TestRecorderDoesNotExposeWebRTCPorts(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "../..")
	for _, filename := range []string{"docker-compose.yml", "docker-compose.production.yml"} {
		data, err := os.ReadFile(filepath.Join(root, filename))
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Services map[string]struct {
				Ports       []string          `yaml:"ports"`
				Environment map[string]string `yaml:"environment"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		for _, service := range []string{"record-worker", "record-worker-debug"} {
			for _, port := range config.Services[service].Ports {
				if strings.Contains(port, "WEBRTC_") || strings.Contains(port, "50000") || !strings.HasPrefix(port, "127.0.0.1:") {
					t.Errorf("%s %s publishes an unexpected port: %s", filename, service, port)
				}
			}
			for key := range config.Services[service].Environment {
				if key == "WEBRTC_UDP_PORT" || key == "WEBRTC_TCP_PORT" || key == "WEBRTC_NAT_IPS" {
					t.Errorf("%s %s configures retired media listener %s", filename, service, key)
				}
			}
		}
		ports := strings.Join(config.Services["media-worker"].Ports, " ")
		if !strings.Contains(ports, "MEDIA_UDP_PORT") || !strings.Contains(ports, "MEDIA_TCP_PORT") {
			t.Errorf("%s must retain SFU WebRTC ports", filename)
		}
	}
}
