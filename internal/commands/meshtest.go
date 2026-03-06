package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

const (
	defaultMeshTestConfig = ".mesh-test.json"
	defaultBroadcastCount = 10
	defaultSpacingSeconds = 15
	defaultWaitSeconds    = 15
	meshConnectTimeout    = 8 * time.Second
	meshPerRequestTimeout = 10 * time.Second
	meshTrafficTimeout    = 30 * time.Second

	pktTypeDeliveryReceipt = 0x07
	pktTypeData            = 0x0A
	pktTypeCoded           = 0x11
)

type meshTestConfig struct {
	Sender         string               `json:"sender"`
	Nodes          []meshTestConfigNode `json:"nodes"`
	Broadcasts     int                  `json:"broadcasts"`
	SpacingSeconds int                  `json:"spacing_seconds"`
	WaitSeconds    int                  `json:"wait_seconds"`
}

type meshTestConfigNode struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
}

type meshNode struct {
	Name         string `json:"name,omitempty"`
	Transport    string `json:"transport"`
	Address      string `json:"address,omitempty"`
	Hardware     string `json:"hardware,omitempty"`
	Reachable    bool   `json:"reachable"`
	Sender       bool   `json:"sender,omitempty"`
	Error        string `json:"error,omitempty"`
	FromConfig   bool   `json:"from_config,omitempty"`
	AutoDetected bool   `json:"auto_detected,omitempty"`
}

type receiptResult struct {
	Recipient string `json:"recipient"`
	LatencyMs int64  `json:"latency_ms"`
}

type broadcastResult struct {
	Index             int             `json:"index"`
	BroadcastID       string          `json:"broadcast_id,omitempty"`
	Expected          int             `json:"expected"`
	Received          int             `json:"received"`
	Recipients        []receiptResult `json:"recipients,omitempty"`
	MissingRecipients []string        `json:"missing_recipients,omitempty"`
}

type nodeReliability struct {
	Address  string `json:"address"`
	Received int    `json:"received"`
	Expected int    `json:"expected"`
}

type primitiveRecipientResult struct {
	Recipient       string `json:"recipient"`
	BroadcastRx     bool   `json:"broadcast_rx"`
	ReceiptTx       bool   `json:"receipt_tx"`
	SenderReceiptRx bool   `json:"sender_receipt_rx"`
}

type primitiveBroadcastResult struct {
	Index        int                        `json:"index"`
	SenderDataTx bool                       `json:"sender_data_tx"`
	Recipients   []primitiveRecipientResult `json:"recipients"`
}

type meshTestReport struct {
	Nodes                      []meshNode                 `json:"nodes"`
	BroadcastsSent             int                        `json:"broadcasts_sent"`
	Broadcasts                 []broadcastResult          `json:"broadcasts"`
	TotalExpected              int                        `json:"total_expected_receipts"`
	TotalReceived              int                        `json:"total_received_receipts"`
	DeliveryRate               float64                    `json:"delivery_rate"`
	NodeReliability            []nodeReliability          `json:"node_reliability"`
	SenderTransport            string                     `json:"sender_transport,omitempty"`
	PrimitiveValidationEnabled bool                       `json:"primitive_validation_enabled,omitempty"`
	PrimitiveBroadcasts        []primitiveBroadcastResult `json:"primitive_broadcasts,omitempty"`
	GeneratedAt                string                     `json:"generated_at"`
}

type meshConnectedNode struct {
	node   *meshNode
	client *bramble.Client
}

type trafficEventStream struct {
	mu     sync.Mutex
	events []bramble.TrafficEvent
}

func (s *trafficEventStream) append(evt bramble.TrafficEvent) {
	s.mu.Lock()
	s.events = append(s.events, evt)
	s.mu.Unlock()
}

func (s *trafficEventStream) mark() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *trafficEventStream) since(mark int) []bramble.TrafficEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mark < 0 || mark >= len(s.events) {
		if mark >= len(s.events) {
			return nil
		}
		mark = 0
	}
	out := make([]bramble.TrafficEvent, len(s.events)-mark)
	copy(out, s.events[mark:])
	return out
}

func newMeshTestCmd() *cobra.Command {
	var cfgPath string
	var senderOverride string
	var count int
	var spacingSec int
	var waitSec int
	var jsonOut bool
	var verbose bool
	var validatePrimitives bool

	cmd := &cobra.Command{
		Use:   "mesh-test",
		Short: "Run an end-to-end mesh broadcast delivery test across multiple nodes",
		Long: `Discovers mesh nodes over serial + config, validates connectivity, sends repeated broadcasts,
and reports delivery reliability per broadcast and per node.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadMeshTestConfig(cfgPath)
			if err != nil {
				return err
			}
			if senderOverride != "" {
				cfg.Sender = strings.TrimSpace(senderOverride)
			}
			if count > 0 {
				cfg.Broadcasts = count
			}
			if spacingSec >= 0 {
				cfg.SpacingSeconds = spacingSec
			}
			if waitSec > 0 {
				cfg.WaitSeconds = waitSec
			}
			setMeshDefaults(&cfg)

			report, runErr := runMeshTest(cmd.Context(), cfg, verbose, validatePrimitives)
			if jsonOut {
				if err := output.PrintJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), formatMeshTestReport(report))
			}
			return runErr
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", defaultMeshTestConfig, "Config file path")
	cmd.Flags().StringVar(&senderOverride, "sender", "", "Override sender transport")
	cmd.Flags().IntVar(&count, "count", defaultBroadcastCount, "Number of broadcasts")
	cmd.Flags().IntVar(&spacingSec, "spacing", defaultSpacingSeconds, "Seconds between broadcasts")
	cmd.Flags().IntVar(&waitSec, "wait", defaultWaitSeconds, "Seconds to wait for delivery receipts")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output results as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show per-event details")
	cmd.Flags().BoolVar(&validatePrimitives, "validate-primitives", false, "Validate protocol primitives (TX/RX/receipt lifecycle) using traffic debug events")

	return cmd
}

func loadMeshTestConfig(path string) (meshTestConfig, error) {
	cfg := meshTestConfig{}
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("bramble-cli: mesh-test read config %q: %w", path, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("bramble-cli: mesh-test parse config %q: %w", path, err)
	}
	return cfg, nil
}

func setMeshDefaults(cfg *meshTestConfig) {
	if cfg.Broadcasts <= 0 {
		cfg.Broadcasts = defaultBroadcastCount
	}
	if cfg.SpacingSeconds < 0 {
		cfg.SpacingSeconds = 0
	}
	if cfg.WaitSeconds <= 0 {
		cfg.WaitSeconds = defaultWaitSeconds
	}
}

func discoverSerialPorts() []string {
	patterns := []string{"/dev/ttyACM*", "/dev/ttyUSB*"}
	seen := map[string]struct{}{}
	ports := make([]string, 0, 8)
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		for _, m := range matches {
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			ports = append(ports, m)
		}
	}
	return ports
}

func runMeshTest(ctx context.Context, cfg meshTestConfig, verbose bool, validatePrimitives bool) (meshTestReport, error) {
	report := meshTestReport{GeneratedAt: time.Now().Format(time.RFC3339)}

	nodes := buildMeshNodeList(cfg)
	report.Nodes = nodes

	connected := make([]meshConnectedNode, 0, len(nodes))
	defer func() {
		for _, n := range connected {
			_ = n.client.Close()
		}
	}()

	for i := range report.Nodes {
		n := &report.Nodes[i]
		client, status, err := connectAndStatus(ctx, n.Transport)
		if err != nil {
			n.Reachable = false
			n.Error = err.Error()
			continue
		}
		n.Reachable = true
		n.Address = status.Address
		n.Hardware = status.Hardware
		connected = append(connected, meshConnectedNode{node: n, client: client})
	}

	sender := selectSender(cfg.Sender, connected)
	if sender == nil {
		return finalizeReport(report, nil), fmt.Errorf("bramble-cli: mesh-test: no reachable sender node")
	}
	sender.node.Sender = true
	report.SenderTransport = sender.node.Transport

	expectedByAddress := map[string]struct{}{}
	recipientConnByAddr := map[string]*meshConnectedNode{}
	senderAddr := strings.TrimSpace(sender.node.Address)
	for i := range connected {
		n := &connected[i]
		if n.node.Transport == sender.node.Transport {
			continue
		}
		addr := strings.TrimSpace(n.node.Address)
		if addr == "" {
			continue
		}
		/* Exclude sender identity even when it appears on multiple transports (e.g. serial + ws). */
		if senderAddr != "" && addr == senderAddr {
			continue
		}
		expectedByAddress[addr] = struct{}{}
		if _, ok := recipientConnByAddr[addr]; !ok {
			recipientConnByAddr[addr] = n
		}
	}

	/* Broadcast delivery events are emitted on the sender node connection. */
	deliveryEvents := make(chan bramble.BroadcastDelivery, 128)
	sender.client.OnBroadcastDelivery(func(evt bramble.BroadcastDelivery) {
		select {
		case deliveryEvents <- evt:
		default:
		}
	})

	spacing := time.Duration(cfg.SpacingSeconds) * time.Second
	waitWindow := time.Duration(cfg.WaitSeconds) * time.Second
	report.BroadcastsSent = cfg.Broadcasts
	report.Broadcasts = make([]broadcastResult, 0, cfg.Broadcasts)
	report.PrimitiveValidationEnabled = validatePrimitives
	trafficStreams := map[*meshConnectedNode]*trafficEventStream{}
	if validatePrimitives {
		report.PrimitiveBroadcasts = make([]primitiveBroadcastResult, 0, cfg.Broadcasts)
		for i := range connected {
			n := &connected[i]
			stream := &trafficEventStream{}
			trafficStreams[n] = stream
			n.client.OnTrafficEvent(func(evt bramble.TrafficEvent) {
				stream.append(evt)
			})
			enableCtx, enableCancel := context.WithTimeout(ctx, meshPerRequestTimeout)
			if err := ensureTrafficDebugEnabled(enableCtx, n.client); err != nil && verbose {
				fmt.Fprintf(os.Stderr, "mesh-test: warning: traffic debug enable failed for %s: %v\n", n.node.Transport, err)
			}
			enableCancel()
		}
		/* Allow node-side traffic debug config to settle before first measured broadcast. */
		time.Sleep(1200 * time.Millisecond)
	}
	perNode := map[string]int{}
	var firstErr error

	for i := 1; i <= cfg.Broadcasts; i++ {
		drainDeliveryEvents(deliveryEvents)
		senderMark := 0
		recipientMark := map[string]int{}
		if validatePrimitives {
			if stream, ok := trafficStreams[sender]; ok {
				senderMark = stream.mark()
			}
			for addr, conn := range recipientConnByAddr {
				if stream, ok := trafficStreams[conn]; ok {
					recipientMark[addr] = stream.mark()
				}
			}
		}
		text := fmt.Sprintf("mesh-test #%d %d", i, time.Now().Unix())
		sendCtx, cancel := context.WithTimeout(ctx, meshPerRequestTimeout)
		sendRes, err := sender.client.Broadcast(sendCtx, text)
		cancel()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("broadcast %d failed: %w", i, err)
			}
			report.Broadcasts = append(report.Broadcasts, broadcastResult{Index: i, Expected: len(expectedByAddress)})
			if i < cfg.Broadcasts && spacing > 0 {
				time.Sleep(spacing)
			}
			continue
		}

		start := time.Now()
		received := map[string]receiptResult{}
		timer := time.NewTimer(waitWindow)
	waitLoop:
		for {
			if len(received) == len(expectedByAddress) {
				break
			}
			select {
			case evt := <-deliveryEvents:
				if sendRes.BroadcastID != "" && evt.BroadcastID != "" && evt.BroadcastID != sendRes.BroadcastID {
					continue
				}
				addr := strings.ToUpper(strings.TrimSpace(evt.Recipient))
				if addr == "" {
					continue
				}
				if _, expected := expectedByAddress[addr]; !expected {
					continue
				}
				if _, exists := received[addr]; exists {
					continue
				}
				latency := time.Since(start).Milliseconds()
				received[addr] = receiptResult{Recipient: addr, LatencyMs: latency}
				if verbose {
					fmt.Fprintf(os.Stderr, "mesh-test: #%d receipt %s in %dms\n", i, addr, latency)
				}
			case <-timer.C:
				break waitLoop
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		recipients := make([]receiptResult, 0, len(received))
		for _, rr := range received {
			recipients = append(recipients, rr)
		}
		sort.Slice(recipients, func(a, b int) bool { return recipients[a].Recipient < recipients[b].Recipient })
		missing := missingRecipients(expectedByAddress, received)
		for _, rr := range recipients {
			perNode[rr.Recipient]++
		}
		report.Broadcasts = append(report.Broadcasts, broadcastResult{
			Index:             i,
			BroadcastID:       sendRes.BroadcastID,
			Expected:          len(expectedByAddress),
			Received:          len(recipients),
			Recipients:        recipients,
			MissingRecipients: missing,
		})

		if validatePrimitives {
			prim := primitiveBroadcastResult{Index: i}
			delivered := map[string]bool{}
			for _, rr := range recipients {
				delivered[rr.Recipient] = true
			}
			if stream, ok := trafficStreams[sender]; ok {
				senderEvents := stream.since(senderMark)
				prim.SenderDataTx = hasTrafficEventAny(senderEvents, []int{pktTypeData, pktTypeCoded}, true)
			}
			recAddrs := make([]string, 0, len(expectedByAddress))
			for addr := range expectedByAddress {
				recAddrs = append(recAddrs, addr)
			}
			sort.Strings(recAddrs)
			for _, addr := range recAddrs {
				pr := primitiveRecipientResult{Recipient: addr, SenderReceiptRx: delivered[addr]}
				if conn, ok := recipientConnByAddr[addr]; ok {
					if stream, ok := trafficStreams[conn]; ok {
						events := stream.since(recipientMark[addr])
						pr.BroadcastRx = hasTrafficEventAny(events, []int{pktTypeData, pktTypeCoded}, false)
						pr.ReceiptTx = hasTrafficEvent(events, pktTypeDeliveryReceipt, true)
					}
				}
				prim.Recipients = append(prim.Recipients, pr)
			}
			report.PrimitiveBroadcasts = append(report.PrimitiveBroadcasts, prim)
		}

		if i < cfg.Broadcasts && spacing > 0 {
			time.Sleep(spacing)
		}
	}

	return finalizeReport(report, perNode), firstErr
}

func buildMeshNodeList(cfg meshTestConfig) []meshNode {
	byTransport := map[string]meshNode{}

	/* Ensure explicit sender transport is included even if not listed in nodes. */
	if s := strings.TrimSpace(cfg.Sender); s != "" {
		byTransport[s] = meshNode{Name: "sender", Transport: s, FromConfig: true}
	}

	for _, n := range cfg.Nodes {
		t := strings.TrimSpace(n.Transport)
		if t == "" {
			continue
		}
		byTransport[t] = meshNode{Name: strings.TrimSpace(n.Name), Transport: t, FromConfig: true}
	}
	for _, port := range discoverSerialPorts() {
		if _, ok := byTransport[port]; ok {
			continue
		}
		byTransport[port] = meshNode{Transport: port, AutoDetected: true}
	}
	nodes := make([]meshNode, 0, len(byTransport))
	for _, n := range byTransport {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Transport < nodes[j].Transport })
	return nodes
}

func connectAndStatus(parent context.Context, transportPath string) (*bramble.Client, *bramble.StatusResponse, error) {
	t := strings.TrimSpace(transportPath)
	if t == "" {
		return nil, nil, fmt.Errorf("empty transport")
	}
	var tr transport.Transport
	if strings.HasPrefix(strings.ToLower(t), "ws://") || strings.HasPrefix(strings.ToLower(t), "wss://") {
		tr = transport.NewWebSocket(t)
	} else {
		tr = transport.NewSerial(t)
	}
	client := bramble.NewClient(tr)
	connectCtx, cancel := context.WithTimeout(parent, meshConnectTimeout)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	statusCtx, statusCancel := context.WithTimeout(parent, meshPerRequestTimeout)
	defer statusCancel()
	status, err := client.Status(statusCtx)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return client, status, nil
}

func selectSender(preferred string, connected []meshConnectedNode) *meshConnectedNode {
	if len(connected) == 0 {
		return nil
	}
	if preferred != "" {
		for i := range connected {
			if connected[i].node.Transport == preferred {
				return &connected[i]
			}
		}
	}
	for i := range connected {
		if strings.HasPrefix(connected[i].node.Transport, "/dev/tty") {
			return &connected[i]
		}
	}
	return &connected[0]
}

func drainDeliveryEvents(ch <-chan bramble.BroadcastDelivery) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func ensureTrafficDebugEnabled(ctx context.Context, client *bramble.Client) error {
	status, err := client.GetTrafficDebug(ctx)
	if err != nil {
		return err
	}
	if status.Enabled && status.IncludeTx && status.IncludeRx {
		return nil
	}
	enabled := true
	includeTx := true
	includeRx := true
	_, err = client.SetTrafficDebug(ctx, bramble.SetTrafficDebugParams{
		Enabled:   &enabled,
		IncludeTx: &includeTx,
		IncludeRx: &includeRx,
	})
	return err
}

func hasTrafficEvent(events []bramble.TrafficEvent, pktType int, isTx bool) bool {
	for _, evt := range events {
		if evt.PktType == pktType && evt.IsTx == isTx {
			return true
		}
	}
	return false
}

func hasTrafficEventAny(events []bramble.TrafficEvent, pktTypes []int, isTx bool) bool {
	for _, t := range pktTypes {
		if hasTrafficEvent(events, t, isTx) {
			return true
		}
	}
	return false
}

func missingRecipients(expected map[string]struct{}, received map[string]receiptResult) []string {
	missing := make([]string, 0)
	for addr := range expected {
		if _, ok := received[addr]; !ok {
			missing = append(missing, addr)
		}
	}
	sort.Strings(missing)
	return missing
}

func finalizeReport(report meshTestReport, perNode map[string]int) meshTestReport {
	totalExpected := 0
	totalReceived := 0
	expectedEach := 0
	if len(report.Broadcasts) > 0 {
		expectedEach = report.Broadcasts[0].Expected
	}
	for _, b := range report.Broadcasts {
		totalExpected += b.Expected
		totalReceived += b.Received
	}
	report.TotalExpected = totalExpected
	report.TotalReceived = totalReceived
	if totalExpected > 0 {
		report.DeliveryRate = (float64(totalReceived) / float64(totalExpected)) * 100.0
	}
	if perNode != nil {
		rel := make([]nodeReliability, 0, len(perNode))
		for addr, got := range perNode {
			rel = append(rel, nodeReliability{Address: addr, Received: got, Expected: len(report.Broadcasts)})
		}
		sort.Slice(rel, func(i, j int) bool { return rel[i].Address < rel[j].Address })
		report.NodeReliability = rel
	} else if expectedEach > 0 {
		report.NodeReliability = []nodeReliability{}
	}
	return report
}

func formatMeshTestReport(report meshTestReport) string {
	var b strings.Builder
	reachable := 0
	for _, n := range report.Nodes {
		if n.Reachable {
			reachable++
		}
	}
	fmt.Fprintf(&b, "=== Bramble Mesh Delivery Test Report ===\n")
	fmt.Fprintf(&b, "Nodes: %d (%d reachable, %d unreachable)\n", len(report.Nodes), reachable, len(report.Nodes)-reachable)
	for _, n := range report.Nodes {
		label := n.Transport
		if n.Address != "" {
			label = fmt.Sprintf("%s (%s) via %s", n.Address, n.Hardware, n.Transport)
		}
		fmt.Fprintf(&b, "  %s", label)
		if n.Sender {
			fmt.Fprintf(&b, " [sender]")
		}
		if !n.Reachable && n.Error != "" {
			fmt.Fprintf(&b, " [unreachable: %s]", n.Error)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Broadcasts: %d sent\n", report.BroadcastsSent)
	fmt.Fprintf(&b, "Delivery rate: %.0f%% (%d/%d receipts)\n", report.DeliveryRate, report.TotalReceived, report.TotalExpected)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Per-broadcast:")
	for _, row := range report.Broadcasts {
		status := "⚠️"
		if row.Expected == 0 || row.Expected == row.Received {
			status = "✅"
		}
		recipients := make([]string, 0, len(row.Recipients))
		for _, rr := range row.Recipients {
			recipients = append(recipients, rr.Recipient)
		}
		fmt.Fprintf(&b, "  #%d %d/%d %s [%s]", row.Index, row.Received, row.Expected, status, strings.Join(recipients, " "))
		if len(row.MissingRecipients) > 0 {
			fmt.Fprintf(&b, " missing: %s", strings.Join(row.MissingRecipients, " "))
		}
		fmt.Fprintln(&b)
	}
	if len(report.NodeReliability) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Per-node reliability:")
		for _, nr := range report.NodeReliability {
			pct := 0.0
			if nr.Expected > 0 {
				pct = (float64(nr.Received) / float64(nr.Expected)) * 100
			}
			fmt.Fprintf(&b, "  %s %d/%d (%.0f%%)\n", nr.Address, nr.Received, nr.Expected, pct)
		}
	}
	if report.PrimitiveValidationEnabled {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Primitive lifecycle validation:")
		for _, pb := range report.PrimitiveBroadcasts {
			senderState := "miss"
			if pb.SenderDataTx {
				senderState = "ok"
			}
			fmt.Fprintf(&b, "  #%d sender_data_tx=%s\n", pb.Index, senderState)
			for _, pr := range pb.Recipients {
				rx := "miss"
				if pr.BroadcastRx {
					rx = "ok"
				}
				tx := "miss"
				if pr.ReceiptTx {
					tx = "ok"
				}
				srx := "miss"
				if pr.SenderReceiptRx {
					srx = "ok"
				}
				fmt.Fprintf(&b, "    %s rx=%s receipt_tx=%s sender_rx_receipt=%s\n", pr.Recipient, rx, tx, srx)
			}
		}
	}
	return b.String()
}
