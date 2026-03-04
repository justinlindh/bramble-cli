package tui

// NickHitRegion maps a region in rendered scrollback to a node address.
type NickHitRegion struct {
	Row      int // line index in the viewport content
	StartCol int
	EndCol   int    // exclusive
	Address  string // raw hex address for DM
}

// ClickMap tracks clickable regions in the scrollback viewport.
type ClickMap struct {
	Nicks []NickHitRegion
}

// Reset clears all hit regions.
func (cm *ClickMap) Reset() {
	cm.Nicks = cm.Nicks[:0]
}

// AddNick registers a nickname hit region.
func (cm *ClickMap) AddNick(row, startCol, endCol int, addr string) {
	cm.Nicks = append(cm.Nicks, NickHitRegion{
		Row:      row,
		StartCol: startCol,
		EndCol:   endCol,
		Address:  addr,
	})
}

// HitTestNick returns the address at the given viewport row/col, or "".
func (cm *ClickMap) HitTestNick(row, col int) string {
	for _, n := range cm.Nicks {
		if row == n.Row && col >= n.StartCol && col < n.EndCol {
			return n.Address
		}
	}
	return ""
}
