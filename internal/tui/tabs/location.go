// Package tabs contains individual tab models for the Bramble TUI.
package tabs

import (
	"fmt"
	"math"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// LocationModel manages the Location tab state.
type LocationModel struct {
	peers    []bramble.LocationPeer
	ownGPS   *bramble.GpsEvent
	selected int
	width    int
	height   int
	resolver PeerResolver

	// styles
	header   lipgloss.Style
	tableHdr lipgloss.Style
	row      lipgloss.Style
	selRow   lipgloss.Style
	muted    lipgloss.Style
	bold     lipgloss.Style
	good     lipgloss.Style
	warn     lipgloss.Style
}

// SetResolver attaches a peer name resolver to the location tab.
func (m *LocationModel) SetResolver(r PeerResolver) {
	m.resolver = r
}

// NewLocation creates a new LocationModel.
func NewLocation() LocationModel {
	return LocationModel{
		header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF87")),
		tableHdr: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AAAACC")),
		row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCDD")),
		selRow:   lipgloss.NewStyle().Background(lipgloss.Color("#1a1a3a")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("#666688")),
		bold:     lipgloss.NewStyle().Bold(true),
		good:     lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")),
		warn:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")),
	}
}

// SetData updates the location data before rendering.
func (m *LocationModel) SetData(gps *bramble.GpsEvent, peers []bramble.LocationPeer) {
	m.ownGPS = gps
	m.peers = peers
	if m.selected >= len(peers) {
		m.selected = max(0, len(peers)-1)
	}
}

// Init implements tea.Model.
func (m LocationModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m LocationModel) Update(msg tea.Msg) (LocationModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if m.selected < len(m.peers)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "o":
			if m.selected < len(m.peers) {
				peer := m.peers[m.selected]
				if peer.Position != nil {
					url := fmt.Sprintf("https://www.google.com/maps?q=%f,%f", peer.Position.Lat, peer.Position.Lon)
					go func() {
						_ = exec.Command("xdg-open", url).Start()
					}()
				}
			}
		}
	}
	return m, nil
}

// View renders the location tab.
func (m LocationModel) View() string {
	var sb strings.Builder

	// Own GPS header
	sb.WriteString("\n")
	sb.WriteString("  ")
	sb.WriteString(m.header.Render("Own Position"))
	sb.WriteString("\n  ")
	sb.WriteString(m.renderOwnPosition())
	sb.WriteString("\n\n")

	// Peer table
	sb.WriteString("  ")
	sb.WriteString(m.header.Render("Peer Locations"))
	sb.WriteString("\n")
	sb.WriteString(m.renderTable())

	return sb.String()
}

func (m LocationModel) renderOwnPosition() string {
	gps := m.ownGPS
	if gps == nil {
		return m.muted.Render("No GPS data")
	}
	if !gps.Valid {
		switch gps.Event {
		case "searching", "fix_lost":
			return m.warn.Render("GPS: searching...")
		default:
			return m.muted.Render("No GPS")
		}
	}

	lat, lon := gps.Lat, gps.Lon
	latDir := "N"
	if lat < 0 {
		latDir = "S"
		lat = -lat
	}
	lonDir := "E"
	if lon < 0 {
		lonDir = "W"
		lon = -lon
	}

	pos := fmt.Sprintf("Position: %.4f°%s, %.4f°%s  Alt: %dm  Sats: %d",
		lat, latDir, lon, lonDir, gps.AltM, gps.Sats)
	return m.good.Render(pos)
}

// Column widths
const (
	colAddr     = 10
	colName     = 14
	colPosition = 24
	colTier     = 8
	colDist     = 10
	colBearing  = 8
)

func locPadRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func (m LocationModel) renderTable() string {
	var sb strings.Builder

	// Header row
	hdr := "  " + m.tableHdr.Render(
		locPadRight("Address", colAddr)+
			locPadRight("Name", colName)+
			locPadRight("Position", colPosition)+
			locPadRight("Tier", colTier)+
			locPadRight("Distance", colDist)+
			locPadRight("Bearing", colBearing),
	)
	sb.WriteString(hdr)
	sb.WriteString("\n")
	sb.WriteString("  " + m.muted.Render(strings.Repeat("─", colAddr+colName+colPosition+colTier+colDist+colBearing)))
	sb.WriteString("\n")

	if len(m.peers) == 0 {
		sb.WriteString("  ")
		sb.WriteString(m.muted.Render("No peers with location data"))
		sb.WriteString("\n")
		return sb.String()
	}

	for i, peer := range m.peers {
		line := m.renderPeerRow(peer)
		peerName := peer.Name
		if peerName == "" && m.resolver != nil {
			peerName = m.resolver.Resolve(peer.Addr)
		}
		rowStr := "  " + locPadRight(peer.Addr, colAddr) +
			locPadRight(peerName, colName) +
			locPadRight(line.pos, colPosition) +
			locPadRight(peer.Tier, colTier) +
			locPadRight(line.dist, colDist) +
			locPadRight(line.bearing, colBearing)

		if i == m.selected {
			sb.WriteString(m.selRow.Render(rowStr))
		} else {
			sb.WriteString(m.row.Render(rowStr))
		}
		sb.WriteString("\n")
	}

	// Key hint
	sb.WriteString("\n  ")
	sb.WriteString(m.muted.Render("[j/k] Navigate  [o] Open in Maps"))

	return sb.String()
}

type peerRow struct {
	pos     string
	dist    string
	bearing string
}

func (m LocationModel) renderPeerRow(peer bramble.LocationPeer) peerRow {
	r := peerRow{dist: "--", bearing: "--"}

	if peer.Position != nil {
		r.pos = fmt.Sprintf("%.4f, %.4f", peer.Position.Lat, peer.Position.Lon)
	} else if peer.GridSquare != "" {
		r.pos = peer.GridSquare
	} else {
		r.pos = "unknown"
	}

	// Distance and bearing require own GPS fix
	if m.ownGPS != nil && m.ownGPS.Valid && peer.Position != nil {
		dist := haversineKm(m.ownGPS.Lat, m.ownGPS.Lon, peer.Position.Lat, peer.Position.Lon)
		if dist < 1.0 {
			r.dist = fmt.Sprintf("%.0fm", dist*1000)
		} else {
			r.dist = fmt.Sprintf("%.1fkm", dist)
		}
		bearing := bearingDeg(m.ownGPS.Lat, m.ownGPS.Lon, peer.Position.Lat, peer.Position.Lon)
		r.bearing = compassDir(bearing)
	}

	return r
}

// haversineKm returns the great-circle distance in kilometers.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// bearingDeg returns the initial bearing in degrees (0=N, 90=E, ...).
func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	lat1R := lat1 * math.Pi / 180
	lat2R := lat2 * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	x := math.Sin(dLon) * math.Cos(lat2R)
	y := math.Cos(lat1R)*math.Sin(lat2R) - math.Sin(lat1R)*math.Cos(lat2R)*math.Cos(dLon)
	bearing := math.Atan2(x, y) * 180 / math.Pi
	return math.Mod(bearing+360, 360)
}

// compassDir maps a bearing in degrees to a compass label.
func compassDir(deg float64) string {
	dirs := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	idx := int((deg+22.5)/45) % 8
	return dirs[idx]
}


