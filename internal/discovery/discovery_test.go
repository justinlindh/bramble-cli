package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDiscoverMDNS(t *testing.T) {
	orig := listenUDP
	t.Cleanup(func() { listenUDP = orig })

	t.Run("listen error", func(t *testing.T) {
		listenUDP = func(network string, laddr *net.UDPAddr) (udpConn, error) {
			return nil, errors.New("listen boom")
		}
		_, err := DiscoverMDNS(context.Background(), 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "mdns listen") {
			t.Fatalf("expected listen error, got %v", err)
		}
	})

	t.Run("send error", func(t *testing.T) {
		fc := &fakeUDPConn{writeErr: errors.New("send boom")}
		listenUDP = func(network string, laddr *net.UDPAddr) (udpConn, error) { return fc, nil }
		_, err := DiscoverMDNS(context.Background(), 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "mdns send") {
			t.Fatalf("expected send error, got %v", err)
		}
	})

	t.Run("discovers and deduplicates", func(t *testing.T) {
		resp := mdnsPacket(
			rrA("bramble-node.local", [4]byte{192, 168, 1, 77}),
			rrSRV("Bramble._bramble._tcp.local", 9090, "bramble-node.local"),
			rrPTR("_bramble._tcp.local", "Bramble._bramble._tcp.local"),
		)
		fc := &fakeUDPConn{reads: [][]byte{resp, resp}}
		listenUDP = func(network string, laddr *net.UDPAddr) (udpConn, error) { return fc, nil }

		nodes, err := DiscoverMDNS(context.Background(), 20*time.Millisecond)
		if err != nil {
			t.Fatalf("DiscoverMDNS error: %v", err)
		}
		if len(nodes) != 1 {
			t.Fatalf("expected deduped single node, got %+v", nodes)
		}
		if nodes[0].Address != "192.0.2.0:9090" {
			t.Fatalf("unexpected node: %+v", nodes[0])
		}
	})
}

func TestBuildMDNSQuery(t *testing.T) {
	q := buildMDNSQuery("_bramble._tcp.local.")
	if len(q) < 12 {
		t.Fatalf("query too short: %d", len(q))
	}
	if q[4] != 0 || q[5] != 1 {
		t.Fatalf("QDCOUNT not 1: %v %v", q[4], q[5])
	}

	name := decodeDNSName(q, 12)
	if name != "_bramble._tcp.local" {
		t.Fatalf("decoded qname mismatch: %q", name)
	}
	// last 4 bytes are QTYPE PTR and QCLASS IN
	tail := q[len(q)-4:]
	if tail[0] != 0x00 || tail[1] != 0x0c || tail[2] != 0x00 || tail[3] != 0x01 {
		t.Fatalf("unexpected qtype/qclass: %v", tail)
	}
}

func TestEncodeDecodeAndSkipDNSName(t *testing.T) {
	enc := encodeDNSName("bramble-node.local.")
	if got := decodeDNSName(enc, 0); got != "bramble-node.local" {
		t.Fatalf("decode mismatch: %q", got)
	}
	if off := skipDNSName(enc, 0); off != len(enc) {
		t.Fatalf("skip offset mismatch: %d != %d", off, len(enc))
	}

	// Compression pointer: name at offset 0, pointer at offset len(enc).
	data := append([]byte{}, enc...)
	data = append(data, 0xC0, 0x00)
	if got := decodeDNSName(data, len(enc)); got != "bramble-node.local" {
		t.Fatalf("pointer decode mismatch: %q", got)
	}
	if off := skipDNSName(data, len(enc)); off != len(enc)+2 {
		t.Fatalf("pointer skip mismatch: %d", off)
	}
}

func TestDecodeDNSName_MalformedSafety(t *testing.T) {
	// Truncated label length and truncated pointer must not panic.
	if got := decodeDNSName([]byte{5, 'a', 'b'}, 0); got != "" {
		t.Fatalf("expected empty on truncated label, got %q", got)
	}
	if got := decodeDNSName([]byte{0xC0}, 0); got != "" {
		t.Fatalf("expected empty on truncated pointer, got %q", got)
	}
	if off := skipDNSName([]byte{3, 'a', 'b'}, 0); off != 4 {
		t.Fatalf("unexpected skip offset on malformed name: %d", off)
	}
}

func TestDetect(t *testing.T) {
	orig := globPaths
	t.Cleanup(func() { globPaths = orig })

	t.Run("glob error", func(t *testing.T) {
		globPaths = func(pattern string) ([]string, error) { return nil, errors.New("glob boom") }
		_, err := Detect()
		if err == nil || !strings.Contains(err.Error(), "glob /dev/ttyUSB*") {
			t.Fatalf("expected glob error, got %v", err)
		}
	})

	t.Run("no devices", func(t *testing.T) {
		globPaths = func(pattern string) ([]string, error) { return nil, nil }
		_, err := Detect()
		if err == nil || !strings.Contains(err.Error(), "no USB serial devices found") {
			t.Fatalf("expected no devices error, got %v", err)
		}
	})

	t.Run("single device", func(t *testing.T) {
		globPaths = func(pattern string) ([]string, error) {
			if pattern == "/dev/ttyUSB*" {
				return []string{"/dev/ttyUSB0"}, nil
			}
			return nil, nil
		}
		got, err := Detect()
		if err != nil {
			t.Fatalf("Detect error: %v", err)
		}
		if got != "/dev/ttyUSB0" {
			t.Fatalf("unexpected port: %q", got)
		}
	})

	t.Run("multiple devices sorted", func(t *testing.T) {
		globPaths = func(pattern string) ([]string, error) {
			switch pattern {
			case "/dev/ttyUSB*":
				return []string{"/dev/ttyUSB2", "/dev/ttyUSB0"}, nil
			case "/dev/ttyACM*":
				return []string{"/dev/ttyACM1"}, nil
			default:
				return nil, nil
			}
		}
		_, err := Detect()
		if err == nil || !strings.Contains(err.Error(), "multiple USB serial devices") {
			t.Fatalf("expected multiple devices error, got %v", err)
		}
		if !strings.Contains(err.Error(), "/dev/ttyUSB0") || !strings.Contains(err.Error(), "/dev/ttyUSB2") {
			t.Fatalf("expected sorted device list in error, got %v", err)
		}
	})
}

func TestParseMDNSResponse_ValidBrambleRecords(t *testing.T) {
	pkt := mdnsPacket(
		rrA("bramble-node.local", [4]byte{192, 168, 1, 50}),
		rrSRV("Bramble._bramble._tcp.local", 8080, "bramble-node.local"),
		rrPTR("_bramble._tcp.local", "Bramble._bramble._tcp.local"),
	)

	nodes := parseMDNSResponse(pkt)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Hostname != "bramble-node.local" {
		t.Fatalf("hostname mismatch: %+v", n)
	}
	if n.Address != "192.0.2.0:8080" {
		t.Fatalf("address mismatch: %+v", n)
	}
	if n.WSURL != "ws://192.0.2.0:8080/ws" {
		t.Fatalf("wsurl mismatch: %+v", n)
	}
}

func TestParseMDNSResponse_DefaultPortAndFiltering(t *testing.T) {
	pkt := mdnsPacket(
		rrA("printer.local", [4]byte{10, 0, 0, 2}),
		rrA("bramble-1ee6.local", [4]byte{10, 0, 0, 3}),
	)

	nodes := parseMDNSResponse(pkt)
	if len(nodes) != 1 {
		t.Fatalf("expected only bramble host to survive filter, got %d", len(nodes))
	}
	if !strings.Contains(nodes[0].WSURL, "ws://10.0.0.3/ws") {
		t.Fatalf("expected default-port ws URL, got %+v", nodes[0])
	}
}

func TestParseMDNSResponse_MalformedPackets(t *testing.T) {
	if got := parseMDNSResponse([]byte{1, 2, 3}); got != nil {
		t.Fatalf("expected nil for too-short packet, got %+v", got)
	}

	// Header says one answer, but rdlen exceeds packet bounds.
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[6:8], 1) // ANCOUNT
	pkt = append(pkt, encodeDNSName("bramble.local")...)
	pkt = append(pkt,
		0x00, 0x01, // TYPE A
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3c, // TTL
		0x00, 0x10, // RDLEN (too long)
	)
	got := parseMDNSResponse(pkt)
	if len(got) != 0 {
		t.Fatalf("expected empty result on truncated RR, got %+v", got)
	}
}

func mdnsPacket(rrs ...[]byte) []byte {
	h := make([]byte, 12)
	binary.BigEndian.PutUint16(h[6:8], uint16(len(rrs))) // ANCOUNT
	pkt := append([]byte{}, h...)
	for _, rr := range rrs {
		pkt = append(pkt, rr...)
	}
	return pkt
}

func rrA(name string, ip [4]byte) []byte {
	nameB := encodeDNSName(name)
	rr := append([]byte{}, nameB...)
	rr = append(rr,
		0x00, 0x01, // TYPE A
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3c, // TTL
		0x00, 0x04, // RDLEN
		ip[0], ip[1], ip[2], ip[3],
	)
	return rr
}

func rrSRV(name string, port int, target string) []byte {
	nameB := encodeDNSName(name)
	targetB := encodeDNSName(target)
	rdata := []byte{
		0x00, 0x00, // priority
		0x00, 0x00, // weight
		byte(port >> 8), byte(port),
	}
	rdata = append(rdata, targetB...)

	rr := append([]byte{}, nameB...)
	rr = append(rr,
		0x00, 0x21, // TYPE SRV
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3c, // TTL
		byte(len(rdata)>>8), byte(len(rdata)),
	)
	rr = append(rr, rdata...)
	return rr
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

type fakeUDPConn struct {
	reads    [][]byte
	readIdx  int
	writeErr error
}

func (f *fakeUDPConn) WriteToUDP(p []byte, addr *net.UDPAddr) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *fakeUDPConn) ReadFromUDP(p []byte) (int, *net.UDPAddr, error) {
	if f.readIdx >= len(f.reads) {
		return 0, nil, timeoutErr{}
	}
	n := copy(p, f.reads[f.readIdx])
	f.readIdx++
	return n, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5353}, nil
}

func (f *fakeUDPConn) SetReadDeadline(time.Time) error { return nil }
func (f *fakeUDPConn) Close() error                    { return nil }

func rrPTR(name string, target string) []byte {
	nameB := encodeDNSName(name)
	targetB := encodeDNSName(target)
	rr := append([]byte{}, nameB...)
	rr = append(rr,
		0x00, 0x0c, // TYPE PTR
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3c, // TTL
		byte(len(targetB)>>8), byte(len(targetB)),
	)
	rr = append(rr, targetB...)
	return rr
}
