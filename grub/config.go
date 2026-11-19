package grub

import (
	"encoding/json"
	"errors"
	"log"

	"os"
	"sync"
	"time"
)

const (
	ERR_HOST_NOT_FOUND   = "host_not_found"
	ERR_CONFIG_NOT_FOUND = "config_not_found"
)

type Host struct {
	ID            string            `json:"id"`
	IP            string            `json:"ip"`
	BroadcastIP   *string           `json:"broadcastIp,omitempty"`
	MacAddress    string            `json:"macAddress"`
	Configs       map[string]string `json:"configs"`
	CurrentConfig *string
	LastSetAt     *time.Time
	Timeout       uint8 `json:"timeout"`
	ResetAfterGet bool  `json:"resetAfterGet"`
}

type HostConfigs struct {
	mu    sync.Mutex
	Hosts map[string]Host `json:"hosts"`
}

type JsonConfig struct {
	Hosts map[string]Host `json:"hosts"`
}

var hc *HostConfigs

func InitHostConfigs(configPath string) {
	// Parse config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading the config file : %s", err.Error())
	}

	var jsonConfig JsonConfig
	err = json.Unmarshal(data, &jsonConfig)
	if err != nil {
		log.Fatalf("Error parsing the config file : %s", err.Error())
	}

	hc = &HostConfigs{Hosts: jsonConfig.Hosts}

}

// Returns the currentConfig for the host
// Returns an empty string if the host is not found
func GetConfigByIp(clientIp string) (string, func()) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	for id, host := range hc.Hosts {
		if host.IP != clientIp || host.CurrentConfig == nil {
			continue
		}
		if host.Timeout != 0 && host.LastSetAt != nil &&
			time.Since(*host.LastSetAt) > time.Duration(host.Timeout)*time.Second {
			return "", func() {}
		}
		selection := host.CurrentConfig
		return host.Configs[*selection], func() {
			hc.mu.Lock()
			defer hc.mu.Unlock()
			current := hc.Hosts[id]
			if current.ResetAfterGet && current.CurrentConfig == selection {
				current.CurrentConfig = nil
				hc.Hosts[id] = current
			}
		}
	}
	return "", func() {}
}

func SetCurrentConfig(hostId string, configName string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	host, ok := hc.Hosts[hostId]
	if !ok {
		return errors.New(ERR_HOST_NOT_FOUND)
	}

	_, ok = host.Configs[configName]
	if !ok {
		return errors.New(ERR_CONFIG_NOT_FOUND)
	}

	host.CurrentConfig = &configName
	t := time.Now()
	host.LastSetAt = &t

	hc.Hosts[hostId] = host

	return nil
}

func GetHostById(hostId string) (*Host, error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	host, ok := hc.Hosts[hostId]
	if !ok {
		return nil, errors.New(ERR_HOST_NOT_FOUND)
	}
	return &host, nil
}
