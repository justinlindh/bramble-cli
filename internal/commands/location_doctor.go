package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// Verdicts a target or the node as a whole can land on. They are ordered by
// severity so a report can be summarised by its worst finding.
const (
	verdictOK    = "ok"
	verdictWarn  = "warn"
	verdictBlock = "blocked"
)

// locationDoctorInput is everything the diagnosis reads. Gathering is separated
// from judging so the judgement is a pure function: the rules below are the
// part worth testing, and they are exactly the rules that were absent when a
// node sat with sharing enabled and shared nothing.
type locationDoctorInput struct {
	Config    *bramble.LocationConfig
	GPS       *bramble.GPSPosition
	GPSErr    error
	Sessions  []bramble.DmSession
	SessErr   error
	Neighbors []bramble.Neighbor
	Routes    []bramble.Route
}

// locationTarget is one configured destination and whether it can be reached.
type locationTarget struct {
	Kind      string `json:"kind"` // "contact" or "channel"
	Target    string `json:"target"`
	Enabled   bool   `json:"enabled"`
	Tier      string `json:"tier,omitempty"`
	IntervalS int    `json:"interval_s,omitempty"`
	Verdict   string `json:"verdict"`
	Reason    string `json:"reason"`
}

// locationReport is the whole diagnosis, and the --json shape.
type locationReport struct {
	Verdict          string           `json:"verdict"`
	Summary          string           `json:"summary"`
	SharingEnabled   bool             `json:"sharing_enabled"`
	DefaultTier      string           `json:"default_tier,omitempty"`
	DefaultIntervalS int              `json:"default_interval_s,omitempty"`
	Source           string           `json:"source,omitempty"`
	SelfPosition     string           `json:"self_position"`
	Targets          []locationTarget `json:"targets"`
	Findings         []string         `json:"findings"`
}

func newLocationDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose why a node is or is not sharing its location",
		Long: `Answer the question "why is this node not sharing location".

The share switch is a permission, not an activity: a node with sharing on and
no usable target transmits nothing, and reading the config back proves only
that a target was accepted, not that anything can be sent to it. This checks
the whole chain in one pass:

  - is sharing enabled, at what tier and interval
  - does a self position resolve (live GPS, else stored manual coordinates)
  - what per-contact and per-channel targets are configured
  - for each contact target, whether a DM session exists

That last one is the failure that hides: a per-contact share is unicast under
a DM session key and is silently dropped when there is no session, logging only
to the serial console. A per-channel target is broadcast under the channel key
and needs no session or route, so the two kinds fail for different reasons and
are reported separately.

Examples:
  bramble location doctor
  bramble location doctor --json`,
		RunE: runLocationDoctor,
	}
}

func runLocationDoctor(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	in, err := gatherLocationDoctorInput(ctx, client)
	if err != nil {
		return err
	}
	report := diagnoseLocation(in)

	if flagJSON {
		return output.PrintJSON(cmd.OutOrStdout(), report)
	}
	printLocationReport(cmd, report)
	return nil
}

func gatherLocationDoctorInput(ctx context.Context, client *bramble.Client) (locationDoctorInput, error) {
	cfg, err := client.Config(ctx)
	if err != nil {
		return locationDoctorInput{}, fmt.Errorf("bramble-cli: get config: %w", err)
	}
	in := locationDoctorInput{Config: &cfg.Location}

	// The rest are individually optional. A board with no GPS, or firmware
	// without getDmSessions, still yields a useful report; recording the error
	// and saying so beats refusing to diagnose anything.
	if pos, err := client.GPSPosition(ctx); err != nil {
		in.GPSErr = err
	} else {
		in.GPS = pos
	}
	if sessions, err := client.DmSessions(ctx); err != nil {
		in.SessErr = err
	} else {
		in.Sessions = sessions.Sessions
	}
	if neighbors, err := client.Neighbors(ctx); err == nil {
		in.Neighbors = neighbors
	}
	if routes, err := client.Routes(ctx); err == nil {
		in.Routes = routes
	}
	return in, nil
}

// diagnoseLocation turns the gathered state into a verdict. Pure: no I/O, so
// every rule here is directly testable against a constructed node state.
func diagnoseLocation(in locationDoctorInput) locationReport {
	r := locationReport{Verdict: verdictOK, Targets: []locationTarget{}, Findings: []string{}}
	if in.Config == nil {
		r.Verdict = verdictBlock
		r.Summary = "the node returned no location configuration"
		return r
	}
	cfg := in.Config

	r.SharingEnabled = cfg.Enabled != nil && *cfg.Enabled
	if cfg.DefaultTier != nil {
		r.DefaultTier = *cfg.DefaultTier
	}
	if cfg.IntervalS != nil {
		r.DefaultIntervalS = *cfg.IntervalS
	}
	if cfg.Source != nil {
		r.Source = *cfg.Source
	}

	selfPosition, hasPosition := describeSelfPosition(in)
	r.SelfPosition = selfPosition

	sessions := sessionsByAddress(in.Sessions)
	reachable := directlyReachable(in.Neighbors, in.Routes)
	for _, rule := range cfg.ContactRules {
		r.Targets = append(r.Targets, judgeContactTarget(rule, sessions, in.SessErr, reachable))
	}
	for _, target := range cfg.ChannelTargets {
		r.Targets = append(r.Targets, judgeChannelTarget(target, in.Neighbors))
	}
	sort.SliceStable(r.Targets, func(a, b int) bool { return r.Targets[a].Kind < r.Targets[b].Kind })

	// Node-level findings, worst first.
	if !r.SharingEnabled {
		r.Verdict = verdictBlock
		r.Findings = append(r.Findings, "location sharing is disabled: nothing is transmitted regardless of targets")
	}

	// Two counts, because they answer different questions. usable is how many
	// targets are fully healthy, which is what the summary reports. transmitting
	// is how many are still putting frames on the air: a channel target nobody
	// is in range to hear is a warning, not a blocker, and folding it in with a
	// target that sends nothing would send someone to reconfigure a node that
	// is working.
	usable, transmitting := 0, 0
	for _, t := range r.Targets {
		if !t.Enabled {
			continue
		}
		if t.Verdict == verdictOK {
			usable++
		}
		if t.Verdict != verdictBlock {
			transmitting++
		}
	}
	switch {
	case len(r.Targets) == 0:
		r.Verdict = worst(r.Verdict, verdictBlock)
		r.Findings = append(r.Findings,
			"no targets configured: the share switch is a permission, not an activity, so a node with no target sends nothing")
	case transmitting == 0:
		r.Verdict = worst(r.Verdict, verdictBlock)
		r.Findings = append(r.Findings, "no configured target can currently be transmitted to (see the target table)")
	}

	if !hasPosition {
		r.Verdict = worst(r.Verdict, verdictBlock)
		r.Findings = append(r.Findings,
			"no self position resolves: with no GPS fix and no stored manual coordinates there is nothing to share")
	}

	if in.SessErr != nil {
		r.Verdict = worst(r.Verdict, verdictWarn)
		r.Findings = append(r.Findings,
			"DM session state is unavailable ("+in.SessErr.Error()+"), so contact targets could not be checked for reachability")
	}

	for _, t := range r.Targets {
		if t.Verdict != verdictOK {
			r.Verdict = worst(r.Verdict, verdictWarn)
			break
		}
	}

	r.Summary = summarise(r, usable)
	return r
}

// describeSelfPosition mirrors the firmware's own resolution order: live GPS
// first, stored manual coordinates as the fallback for GPS-less boards.
func describeSelfPosition(in locationDoctorInput) (string, bool) {
	if in.GPS != nil && in.GPS.Valid {
		return "GPS fix", true
	}
	if in.Config != nil && in.Config.Lat != nil && in.Config.Lon != nil {
		return "manual coordinates (no GPS fix)", true
	}
	if in.GPSErr != nil {
		return "none (no manual coordinates; GPS unavailable: " + in.GPSErr.Error() + ")", false
	}
	return "none (no GPS fix and no manual coordinates)", false
}

func sessionsByAddress(sessions []bramble.DmSession) map[string]bramble.DmSession {
	m := make(map[string]bramble.DmSession, len(sessions))
	for _, s := range sessions {
		m[strings.ToUpper(s.Address)] = s
	}
	return m
}

// judgeContactTarget decides whether a per-contact rule can currently transmit.
// A contact share is unicast under a DM session key, so no session means the
// share is dropped with nothing but a console line to show for it.
func judgeContactTarget(rule bramble.LocationContactRule, sessions map[string]bramble.DmSession, sessErr error, reachable map[string]bool) locationTarget {
	t := locationTarget{
		Kind:      "contact",
		Target:    strings.ToUpper(rule.Address),
		Enabled:   rule.Enabled == nil || *rule.Enabled,
		Tier:      rule.Tier,
		IntervalS: derefInt(rule.IntervalS),
	}
	if !t.Enabled {
		t.Verdict = verdictWarn
		t.Reason = "rule is disabled"
		return t
	}
	if sessErr != nil {
		t.Verdict = verdictWarn
		t.Reason = "session state unavailable"
		return t
	}

	session, ok := sessions[t.Target]
	switch {
	case !ok:
		t.Verdict = verdictBlock
		t.Reason = "no DM session: directed shares to this peer are dropped silently. Send this peer a DM to establish one"
	case !session.Active():
		t.Verdict = verdictBlock
		t.Reason = "session is " + session.State + ", not active: shares are dropped until the key exchange completes"
	case !session.RatchetValid:
		t.Verdict = verdictBlock
		t.Reason = "session has no send chain yet: nothing can be encrypted to this peer"
	case !reachable[t.Target]:
		// The session is fine, but a unicast still needs somewhere to go. This
		// is a warning rather than a blocker because a route is built on demand
		// and a neighbor can come back into range without anything changing on
		// this node.
		t.Verdict = verdictWarn
		t.Reason = "active session, but the peer is not a neighbor and has no route: the share has nowhere to go right now"
	default:
		t.Verdict = verdictOK
		t.Reason = "active session"
		if !session.Verified {
			t.Reason = "active session (peer identity unverified)"
		}
	}
	return t
}

// directlyReachable is the set of peer addresses this node can currently put a
// unicast on the air for: a direct radio neighbor, or a destination with a
// route that is not broken.
func directlyReachable(neighbors []bramble.Neighbor, routes []bramble.Route) map[string]bool {
	m := make(map[string]bool, len(neighbors)+len(routes))
	for _, n := range neighbors {
		m[strings.ToUpper(n.Address)] = true
	}
	for _, r := range routes {
		if r.State == "broken" {
			continue
		}
		m[strings.ToUpper(r.Dest)] = true
	}
	return m
}

// judgeChannelTarget decides whether a per-channel target can transmit. A
// channel share is one broadcast under the channel key: no session, no route
// and no prior directed traffic are needed, so the only thing that can stop it
// is the rule being off or nobody being in radio range to hear it.
func judgeChannelTarget(target bramble.LocationChannelTarget, neighbors []bramble.Neighbor) locationTarget {
	t := locationTarget{
		Kind:      "channel",
		Target:    fmt.Sprintf("channel %d", target.Channel),
		Enabled:   target.Enabled == nil || *target.Enabled,
		Tier:      target.Tier,
		IntervalS: derefInt(target.IntervalS),
	}
	if !t.Enabled {
		t.Verdict = verdictWarn
		t.Reason = "target is disabled"
		return t
	}
	if len(neighbors) == 0 {
		// Still transmitting, so this is not a blocker: the share goes out and
		// nobody is in range to hear it, which is a different problem from the
		// node being silent.
		t.Verdict = verdictWarn
		t.Reason = "broadcasting, but no neighbors are in radio range to receive it"
		return t
	}
	t.Verdict = verdictOK
	t.Reason = fmt.Sprintf("broadcast under the channel key, no session needed (%d neighbor(s) in range)", len(neighbors))
	return t
}

func summarise(r locationReport, usable int) string {
	if !r.SharingEnabled {
		return "not sharing: location sharing is turned off"
	}
	switch r.Verdict {
	case verdictBlock:
		return "not sharing: sharing is on but nothing is being transmitted"
	case verdictWarn:
		return fmt.Sprintf("sharing to %d of %d target(s), with problems", usable, len(r.Targets))
	default:
		return fmt.Sprintf("sharing to %d target(s)", usable)
	}
}

// worst returns the more severe of two verdicts.
func worst(a, b string) string {
	rank := map[string]int{verdictOK: 0, verdictWarn: 1, verdictBlock: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func printLocationReport(cmd *cobra.Command, r locationReport) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Verdict:       %s\n", strings.ToUpper(r.Verdict))
	fmt.Fprintf(w, "Summary:       %s\n", r.Summary)
	fmt.Fprintf(w, "Sharing:       %s\n", boolStr(r.SharingEnabled, "enabled", "disabled"))
	if r.DefaultTier != "" {
		fmt.Fprintf(w, "Default tier:  %s\n", r.DefaultTier)
	}
	if r.DefaultIntervalS > 0 {
		fmt.Fprintf(w, "Interval:      %ds\n", r.DefaultIntervalS)
	}
	if r.Source != "" {
		fmt.Fprintf(w, "Source:        %s\n", r.Source)
	}
	fmt.Fprintf(w, "Self position: %s\n", r.SelfPosition)
	fmt.Fprintln(w)

	if len(r.Targets) == 0 {
		fmt.Fprintln(w, "Targets:       none configured")
	} else {
		rows := make([][]string, len(r.Targets))
		for i, t := range r.Targets {
			rows[i] = []string{
				t.Kind,
				t.Target,
				boolStr(t.Enabled, "yes", "no"),
				dashIfEmpty(t.Tier),
				intervalCell(t.IntervalS),
				strings.ToUpper(t.Verdict),
				t.Reason,
			}
		}
		output.Table(w, []string{"KIND", "TARGET", "ENABLED", "TIER", "INTERVAL", "VERDICT", "DETAIL"}, rows)
	}

	if len(r.Findings) > 0 {
		fmt.Fprintln(w)
		for _, f := range r.Findings {
			fmt.Fprintf(w, "  * %s\n", f)
		}
	}
}

func intervalCell(s int) string {
	if s <= 0 {
		return "default"
	}
	return fmt.Sprintf("%ds", s)
}
