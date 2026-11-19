package tftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/bducha/mbwol/grub"
)

const (
	TFTP_PORT               = 69
	BLOCK_SIZE              = 512
	PACKET_SIZE             = 516
	OPCODE_RRQ              = 1
	OPCODE_DATA             = 3
	OPCODE_ACK              = 4
	OPCODE_ERROR            = 5
	OPCODE_OACK             = 6
	ERROR_CODE_ILLEGAL_TFTP = 4
)

type TFTPPacket struct {
	Opcode  int
	Payload []byte
}

type Connection struct {
	Conn      *net.UDPConn
	BlockSize int
	DataLen   int64
}

func ListenAndServeTFTP() error {

	addr := &net.UDPAddr{Port: TFTP_PORT, IP: net.ParseIP("0.0.0.0")}
	conn, err := net.ListenUDP("udp", addr)

	if err != nil {
		return fmt.Errorf("error listening on UDP port : %s", err)
	}

	defer conn.Close()

	slog.Info("TFTP Server listening", "addr", conn.LocalAddr().String())

	for {
		buffer := make([]byte, PACKET_SIZE)
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			slog.Error("Error reading from UDP", "err", err.Error())
			continue
		}

		slog.Debug("Received packet", "from", clientAddr.String(), "size", n)
		if n < 2 {
			continue
		}
		opcode := int(binary.BigEndian.Uint16(buffer[:2]))
		switch opcode {
		case OPCODE_RRQ:
			go handleRRQ(buffer[:n], clientAddr)
		default:
			slog.Warn("Unsupported opcode", "opcode", opcode)
		}
	}
}

func handleRRQ(packet []byte, clientAddr *net.UDPAddr) {
	parts := bytes.Split(packet[2:], []byte{0})
	if len(parts) < 3 || len(parts[len(parts)-1]) != 0 || len(parts)%2 == 0 {
		return
	}
	content, complete := grub.GetConfigByIp(clientAddr.IP.String())
	data := []byte(content)
	udpConn, err := net.DialUDP("udp", nil, clientAddr)
	conn := Connection{
		Conn:      udpConn,
		BlockSize: BLOCK_SIZE,
		DataLen:   int64(len(data)),
	}
	if err != nil {
		slog.Error("Error dialing UDP", "err", err.Error(), "client", clientAddr.String())
		return
	}
	defer udpConn.Close()

	// Handle options
	err = handleOptions(parts[2:len(parts)-1], &conn)
	if err == nil {
		err = sendData(data, &conn)
	}
	if err != nil {
		slog.Debug("TFTP transfer incomplete", "err", err)
		return
	}
	complete()
}

func handleOptions(options [][]byte, conn *Connection) error {
	packet := []byte{0, OPCODE_OACK}
	for i := 0; i < len(options); i += 2 {
		name, value := string(options[i]), string(options[i+1])
		switch name {
		case "tsize":
			value = strconv.FormatInt(conn.DataLen, 10)
		case "blksize":
			size, err := strconv.Atoi(value)
			if err != nil || size < 8 || size > 65464 {
				continue
			}
			conn.BlockSize = size
		default:
			continue
		}
		packet = append(packet, name...)
		packet = append(packet, 0)
		packet = append(packet, value...)
		packet = append(packet, 0)
	}
	if len(packet) == 2 {
		return nil
	}
	if _, err := conn.Conn.Write(packet); err != nil {
		return err
	}
	return expectACK(conn.Conn, 0)
}

func expectACK(conn *net.UDPConn, block uint16) error {
	packet, err := receivePacket(conn)
	if err != nil {
		return err
	}
	if packet.Opcode != OPCODE_ACK || len(packet.Payload) != 2 ||
		binary.BigEndian.Uint16(packet.Payload) != block {
		return fmt.Errorf("unexpected TFTP response for block %d", block)
	}
	return nil
}

func sendData(data []byte, conn *Connection) error {
	for block := uint16(1); ; block++ {
		size := min(len(data), conn.BlockSize)
		if _, err := conn.Conn.Write(createDataPacket(int(block), data[:size])); err != nil {
			return err
		}
		if err := expectACK(conn.Conn, block); err != nil {
			return err
		}
		if size < conn.BlockSize {
			return nil
		}
		data = data[size:]
	}
}

func createDataPacket(blockNumber int, data []byte) []byte {
	opCode := []byte{0, byte(OPCODE_DATA)}
	blockNum := []byte{byte(blockNumber >> 8), byte(blockNumber)}
	packet := append(opCode, blockNum...)
	packet = append(packet, data...)
	return packet
}

func receivePacket(conn *net.UDPConn) (*TFTPPacket, error) {
	err := conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 516)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			// Handle timeout error
			return nil, errors.New("read operation timed out")
		}
		return nil, err
	}

	// Clear the deadline
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, err
	}

	if n < 2 {
		return nil, errors.New("short TFTP response")
	}
	opcode := int(binary.BigEndian.Uint16(buffer[:2]))
	payload := buffer[2:n]
	return &TFTPPacket{Opcode: opcode, Payload: payload}, nil
}
