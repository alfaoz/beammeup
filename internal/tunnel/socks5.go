package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	socks5Version = 0x05
	authNone      = 0x00
	authNoAccept  = 0xFF
	cmdConnect    = 0x01
	atypIPv4      = 0x01
	atypDomain    = 0x03
	atypIPv6      = 0x04
	repSuccess    = 0x00
	repFailure    = 0x01
	repNotAllowed = 0x02
	repHostUnreach = 0x04
)

// DialFunc dials a network address. Typically backed by ssh.Client.Dial.
type DialFunc func(network, addr string) (net.Conn, error)

// HandleConn processes a single SOCKS5 connection. dialFn is called to
// establish the outbound connection (through the SSH tunnel).
func HandleConn(conn net.Conn, dialFn DialFunc) error {
	defer conn.Close()

	// --- auth negotiation ---
	// +----+----------+----------+
	// |VER | NMETHODS | METHODS  |
	// +----+----------+----------+
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read auth header: %w", err)
	}
	if header[0] != socks5Version {
		return errors.New("unsupported SOCKS version")
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read auth methods: %w", err)
	}

	hasNoAuth := false
	for _, m := range methods {
		if m == authNone {
			hasNoAuth = true
			break
		}
	}
	if !hasNoAuth {
		conn.Write([]byte{socks5Version, authNoAccept})
		return errors.New("client does not support no-auth")
	}
	if _, err := conn.Write([]byte{socks5Version, authNone}); err != nil {
		return fmt.Errorf("write auth response: %w", err)
	}

	// --- request ---
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if req[0] != socks5Version {
		return errors.New("bad SOCKS version in request")
	}
	if req[1] != cmdConnect {
		sendReply(conn, repNotAllowed, nil)
		return fmt.Errorf("unsupported command: %d", req[1])
	}

	var host string
	switch req[3] {
	case atypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return fmt.Errorf("read ipv4 addr: %w", err)
		}
		host = net.IP(addr).String()
	case atypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return fmt.Errorf("read ipv6 addr: %w", err)
		}
		host = net.IP(addr).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("read domain length: %w", err)
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return fmt.Errorf("read domain: %w", err)
		}
		host = string(domain)
	default:
		sendReply(conn, repFailure, nil)
		return fmt.Errorf("unsupported address type: %d", req[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)
	target := net.JoinHostPort(host, strconv.Itoa(int(port)))

	// --- connect via tunnel ---
	remote, err := dialFn("tcp", target)
	if err != nil {
		sendReply(conn, repHostUnreach, nil)
		return fmt.Errorf("dial %s: %w", target, err)
	}
	defer remote.Close()

	sendReply(conn, repSuccess, remote.LocalAddr())

	// --- bidirectional pipe ---
	done := make(chan struct{}, 2)
	go func() { io.Copy(remote, conn); done <- struct{}{} }()
	go func() { io.Copy(conn, remote); done <- struct{}{} }()
	<-done
	return nil
}

func sendReply(conn net.Conn, rep byte, bindAddr net.Addr) {
	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	reply := []byte{socks5Version, rep, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
	if bindAddr != nil {
		if tcpAddr, ok := bindAddr.(*net.TCPAddr); ok {
			ip4 := tcpAddr.IP.To4()
			if ip4 != nil {
				copy(reply[4:8], ip4)
			}
			binary.BigEndian.PutUint16(reply[8:10], uint16(tcpAddr.Port))
		}
	}
	conn.Write(reply)
}
