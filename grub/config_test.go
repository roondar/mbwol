package grub

import "testing"

func TestSelectionAcknowledgment(t *testing.T) {
	hc = &HostConfigs{Hosts: map[string]Host{"desktop": {
		IP: "192.0.2.10", Configs: map[string]string{"linux": "linux", "windows": "windows"},
		ResetAfterGet: true,
	}}}
	if err := SetCurrentConfig("desktop", "linux"); err != nil {
		t.Fatal(err)
	}
	first, old := GetConfigByIp("192.0.2.10")
	second, _ := GetConfigByIp("192.0.2.10")
	if first != "linux" || second != first {
		t.Fatal("pending selection lost before acknowledgment")
	}
	if err := SetCurrentConfig("desktop", "windows"); err != nil {
		t.Fatal(err)
	}
	old()
	current, complete := GetConfigByIp("192.0.2.10")
	if current != "windows" {
		t.Fatal("stale completion cleared newer selection")
	}
	complete()
	if got, _ := GetConfigByIp("192.0.2.10"); got != "" {
		t.Fatal("acknowledged selection not cleared")
	}
}
