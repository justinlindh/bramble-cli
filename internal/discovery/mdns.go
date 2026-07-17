// Package discovery provides device discovery for Bramble nodes.
package discovery

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

type udpConn interface {
	WriteToUDP([]byte, *net.UDPAddr) (int, error)
	ReadFromUDP([]byte) (int, *net.UDPAddr, error)
	SetReadDeadline(time.Time) error
	Close() error
}

var listenUDP = func(network string, laddr *net.UDPAddr) (udpConn, error) {
	return net.ListenUDP(network, laddr)
}

// Node represents a Bramble node discovered via mDNS.
type Node struct {
	Hostname string // e.g. "bramble-1ee6"
	Name     string // instance name, e.g. "Bramble Mesh Node"
	Address  string // IP:port
	WSURL    string // WebSocket URL, e.g. "ws://192.0.2.64/ws"
}

// DiscoverMDNS scans the local network for Bramble nodes advertising _bramble._tcp
// using a simple DNS-SD multicast query. Returns discovered nodes after the timeout.
func DiscoverMDNS(ctx context.Context, timeout time.Duration) ([]Node, error) {
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Multicast DNS address
	mdnsAddr := &net.UDPAddr{
		IP:   net.IPv4(224, 0, 0, 251),
		Port: 5353,
	}

	conn, err := listenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("mdns listen: %w", err)
	}
	defer conn.Close() //nolint:errcheck // UDP conn close in CLI

	// Build DNS query for _bramble._tcp.local PTR records
	query := buildMDNSQuery("_bramble._tcp.local.")
	if _, err := conn.WriteToUDP(query, mdnsAddr); err != nil {
		return nil, fmt.Errorf("mdns send: %w", err)
	}

	var nodes []Node
	seen := make(map[string]bool)
	buf := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			sort.Slice(nodes, func(i, j int) bool {
				return nodes[i].Hostname < nodes[j].Hostname
			})
			return nodes, nil
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		// Parse responses looking for _bramble._tcp answers with A records
		parsed := parseMDNSResponse(buf[:n])
		for _, p := range parsed {
			key := p.Address
			if seen[key] {
				continue
			}
			seen[key] = true
			nodes = append(nodes, p)
		}
	}
}

// buildMDNSQuery constructs a minimal DNS query packet for a PTR record.
func buildMDNSQuery(name string) []byte {
	// DNS header: ID=0, QR=0, QDCOUNT=1
	header := []byte{
		0x00, 0x00, // ID
		0x00, 0x00, // Flags (standard query)
		0x00, 0x01, // QDCOUNT = 1
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}

	// Encode QNAME
	qname := encodeDNSName(name)

	// QTYPE=PTR (12), QCLASS=IN (1) with unicast-response bit
	qtype := []byte{0x00, 0x0c, 0x00, 0x01}

	pkt := make([]byte, 0, len(header)+len(qname)+len(qtype))
	pkt = append(pkt, header...)
	pkt = append(pkt, qname...)
	pkt = append(pkt, qtype...)
	return pkt
}

// encodeDNSName encodes a domain name as DNS wire format.
func encodeDNSName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	parts := strings.Split(name, ".")
	var buf []byte
	for _, p := range parts {
		buf = append(buf, byte(len(p)))
		buf = append(buf, []byte(p)...)
	}
	buf = append(buf, 0x00) // root label
	return buf
}

// parseMDNSResponse does a best-effort parse of mDNS response packets
// looking for _bramble._tcp service instances with A records.
func parseMDNSResponse(data []byte) []Node {
	if len(data) < 12 {
		return nil
	}

	// Extract all A record IPs and SRV hostnames from the response
	var nodes []Node
	names := map[string]string{} // hostname -> instance name
	addrs := map[string]string{} // hostname -> IP
	ports := map[string]int{}    // hostname -> port

	// Simple scan: look for answer/additional sections
	// We use a very simplified parser that looks for recognizable patterns
	offset := 12 // skip header

	// Skip questions
	qdcount := int(data[4])<<8 | int(data[5])
	for i := 0; i < qdcount && offset < len(data); i++ {
		offset = skipDNSName(data, offset)
		offset += 4 // QTYPE + QCLASS
	}

	// Parse answer + authority + additional sections
	totalRR := (int(data[6])<<8 | int(data[7])) +
		(int(data[8])<<8 | int(data[9])) +
		(int(data[10])<<8 | int(data[11]))

	for i := 0; i < totalRR && offset+10 < len(data); i++ {
		nameStr := decodeDNSName(data, offset)
		offset = skipDNSName(data, offset)
		if offset+10 > len(data) {
			break
		}

		rtype := int(data[offset])<<8 | int(data[offset+1])
		// rclass at offset+2..3
		// ttl at offset+4..7
		rdlen := int(data[offset+8])<<8 | int(data[offset+9])
		offset += 10

		if offset+rdlen > len(data) {
			break
		}

		switch rtype {
		case 1: // A record
			if rdlen == 4 {
				ip := fmt.Sprintf("%d.%d.%d.%d", data[offset], data[offset+1], data[offset+2], data[offset+3])
				addrs[nameStr] = ip
			}
		case 33: // SRV record
			if rdlen >= 6 {
				port := int(data[offset+4])<<8 | int(data[offset+5])
				target := decodeDNSName(data, offset+6)
				ports[target] = port
				names[target] = nameStr
			}
		case 12: // PTR record
			// instance name pointing to service
			instance := decodeDNSName(data, offset)
			if strings.Contains(nameStr, "_bramble") {
				names[instance] = instance
			}
		}

		offset += rdlen
	}

	// Correlate: find hostnames that have both A records and ports
	for hostname, ip := range addrs {
		port := 80
		if p, ok := ports[hostname]; ok {
			port = p
		}
		// Only include if this is related to a bramble service
		isBramble := false
		for name := range names {
			if strings.Contains(name, "bramble") || strings.Contains(name, hostname) {
				isBramble = true
				break
			}
		}
		if !isBramble && !strings.Contains(hostname, "bramble") {
			continue
		}

		addr := fmt.Sprintf("%s:%d", ip, port)
		wsURL := fmt.Sprintf("ws://%s:%d/ws", ip, port)
		if port == 80 {
			wsURL = fmt.Sprintf("ws://%s/ws", ip)
		}

		nodes = append(nodes, Node{
			Hostname: strings.TrimSuffix(hostname, ".local."),
			Name:     "Bramble Mesh Node",
			Address:  addr,
			WSURL:    wsURL,
		})
	}

	return nodes
}

// skipDNSName advances past a DNS name (handling compression pointers).
func skipDNSName(data []byte, offset int) int {
	for offset < len(data) {
		if data[offset] == 0 {
			return offset + 1
		}
		if data[offset]&0xC0 == 0xC0 {
			return offset + 2 // compression pointer
		}
		offset += int(data[offset]) + 1
	}
	return offset
}

// decodeDNSName decodes a DNS name from wire format (with compression pointer support).
func decodeDNSName(data []byte, offset int) string {
	var parts []string
	visited := make(map[int]bool)
	for offset < len(data) {
		if visited[offset] {
			break // loop protection
		}
		visited[offset] = true

		if data[offset] == 0 {
			break
		}
		if data[offset]&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				break
			}
			ptr := (int(data[offset]&0x3F) << 8) | int(data[offset+1])
			offset = ptr
			continue
		}
		length := int(data[offset])
		offset++
		if offset+length > len(data) {
			break
		}
		parts = append(parts, string(data[offset:offset+length]))
		offset += length
	}
	return strings.Join(parts, ".")
}
