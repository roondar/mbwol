package tftp

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestNegotiatedTransfer(t *testing.T) {
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := net.DialUDP("udp4", nil, client.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client.SetDeadline(time.Now().Add(3 * time.Second))
	payload := bytes.Repeat([]byte{'a'}, 67)
	result := make(chan error, 1)
	go func() {
		c := &Connection{Conn: server, BlockSize: 512, DataLen: 67}
		err := handleOptions(bytes.Split([]byte("blksize 1024 tsize 0"), []byte(" ")), c)
		if err == nil {
			err = sendData(payload, c)
		}
		result <- err
	}()
	packet := make([]byte, 1200)
	n, peer, err := client.ReadFromUDP(packet)
	if err != nil || !bytes.Equal(packet[:n], []byte("\x00\x06blksize\x001024\x00tsize\x0067\x00")) {
		t.Fatalf("OACK: %x, %v", packet[:n], err)
	}
	if _, err := client.WriteToUDP([]byte{0, 4, 0, 0}, peer); err != nil {
		t.Fatal(err)
	}
	n, peer, err = client.ReadFromUDP(packet)
	if err != nil || !bytes.Equal(packet[:n], createDataPacket(1, payload)) {
		t.Fatalf("DATA: %x, %v", packet[:n], err)
	}
	if _, err := client.WriteToUDP([]byte{0, 4, 0, 1}, peer); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
