package commands

import (
	"errors"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func boolPtr(b bool) *bool        { return &b }
func intPtr(i int) *int           { return &i }
func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }

// sharingNode is the baseline: sharing on, a resolvable position, one contact
// target with a healthy session. Each test perturbs one thing from here, so
// what the test is about is what it changes.
func sharingNode() locationDoctorInput {
	return locationDoctorInput{
		Config: &bramble.LocationConfig{
			Enabled:     boolPtr(true),
			DefaultTier: strPtr("coarse"),
			IntervalS:   intPtr(300),
			ContactRules: []bramble.LocationContactRule{
				{Address: "DEADBEEF", Tier: "full", IntervalS: intPtr(60)},
			},
		},
		GPS: &bramble.GPSPosition{Valid: true, Lat: 1, Lon: 2},
		Sessions: []bramble.DmSession{
			{Address: "DEADBEEF", State: "active", Verified: true, RatchetValid: true},
		},
		Neighbors: []bramble.Neighbor{{Address: "DEADBEEF"}},
	}
}

func findTarget(t *testing.T, r locationReport, name string) locationTarget {
	t.Helper()
	for _, tgt := range r.Targets {
		if tgt.Target == name {
			return tgt
		}
	}
	t.Fatalf("no target %q in report (%d targets)", name, len(r.Targets))
	return locationTarget{}
}

func TestDiagnoseLocationHealthyNodeIsOK(t *testing.T) {
	r := diagnoseLocation(sharingNode())
	if r.Verdict != verdictOK {
		t.Errorf("verdict = %s, want %s (findings: %v)", r.Verdict, verdictOK, r.Findings)
	}
	if got := findTarget(t, r, "DEADBEEF"); got.Verdict != verdictOK {
		t.Errorf("contact target verdict = %s (%s), want ok", got.Verdict, got.Reason)
	}
}

func TestDiagnoseLocationContactWithNoSessionIsBlocked(t *testing.T) {
	// This is the failure the whole command exists for: sharing is on, the
	// target is configured and reads back fine, and every share to it is
	// dropped with only a serial console line to show for it.
	in := sharingNode()
	in.Sessions = nil

	r := diagnoseLocation(in)
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s", r.Verdict, verdictBlock)
	}
	tgt := findTarget(t, r, "DEADBEEF")
	if tgt.Verdict != verdictBlock {
		t.Errorf("target verdict = %s, want %s", tgt.Verdict, verdictBlock)
	}
	if !strings.Contains(tgt.Reason, "no DM session") {
		t.Errorf("reason %q does not name the missing session", tgt.Reason)
	}
	if !strings.Contains(r.Summary, "nothing is being transmitted") {
		t.Errorf("summary %q does not say the node is silent", r.Summary)
	}
}

func TestDiagnoseLocationHandshakingSessionIsBlocked(t *testing.T) {
	// A half-open session is not a usable one. Treating "there is a slot for
	// this peer" as reachable would report the silent node as healthy.
	in := sharingNode()
	in.Sessions = []bramble.DmSession{{Address: "DEADBEEF", State: "handshaking"}}

	r := diagnoseLocation(in)
	tgt := findTarget(t, r, "DEADBEEF")
	if tgt.Verdict != verdictBlock {
		t.Errorf("target verdict = %s, want %s (%s)", tgt.Verdict, verdictBlock, tgt.Reason)
	}
	if !strings.Contains(tgt.Reason, "handshaking") {
		t.Errorf("reason %q does not name the handshaking state", tgt.Reason)
	}
}

func TestDiagnoseLocationActiveSessionWithNoSendChainIsBlocked(t *testing.T) {
	in := sharingNode()
	in.Sessions = []bramble.DmSession{{Address: "DEADBEEF", State: "active", RatchetValid: false}}

	r := diagnoseLocation(in)
	if got := findTarget(t, r, "DEADBEEF"); got.Verdict != verdictBlock {
		t.Errorf("target verdict = %s, want %s (%s)", got.Verdict, verdictBlock, got.Reason)
	}
}

func TestDiagnoseLocationSessionAddressMatchIsCaseInsensitive(t *testing.T) {
	// The firmware emits uppercase hex and a config may hold either case.
	// A case-sensitive match would report every contact as sessionless.
	in := sharingNode()
	in.Config.ContactRules = []bramble.LocationContactRule{{Address: "deadbeef"}}
	in.Sessions = []bramble.DmSession{{Address: "DEADBEEF", State: "active", RatchetValid: true}}

	r := diagnoseLocation(in)
	if got := findTarget(t, r, "DEADBEEF"); got.Verdict != verdictOK {
		t.Errorf("target verdict = %s, want ok (%s)", got.Verdict, got.Reason)
	}
}

func TestDiagnoseLocationSharingDisabledIsBlocked(t *testing.T) {
	in := sharingNode()
	in.Config.Enabled = boolPtr(false)

	r := diagnoseLocation(in)
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s", r.Verdict, verdictBlock)
	}
	if !strings.Contains(strings.Join(r.Findings, " "), "disabled") {
		t.Errorf("findings %v do not say sharing is disabled", r.Findings)
	}
}

func TestDiagnoseLocationSharingOnWithNoTargetsIsBlocked(t *testing.T) {
	// The state the bug hid in: the switch reads "on" and there is nothing to
	// send to, so the node is silent while reporting itself as sharing.
	in := sharingNode()
	in.Config.ContactRules = nil
	in.Config.ChannelTargets = nil

	r := diagnoseLocation(in)
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s", r.Verdict, verdictBlock)
	}
	joined := strings.Join(r.Findings, " ")
	if !strings.Contains(joined, "no targets configured") {
		t.Errorf("findings %v do not name the missing targets", r.Findings)
	}
	if !strings.Contains(joined, "permission, not an activity") {
		t.Errorf("findings %v do not explain why the switch alone means nothing", r.Findings)
	}
}

func TestDiagnoseLocationNoSelfPositionIsBlocked(t *testing.T) {
	in := sharingNode()
	in.GPS = &bramble.GPSPosition{Valid: false}

	r := diagnoseLocation(in)
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s", r.Verdict, verdictBlock)
	}
	if !strings.Contains(strings.Join(r.Findings, " "), "no self position") {
		t.Errorf("findings %v do not name the missing position", r.Findings)
	}
}

func TestDiagnoseLocationManualCoordinatesSatisfyPosition(t *testing.T) {
	// A GPS-less board is configured with stored coordinates, and the firmware
	// falls back to them. Reporting "no position" here would send someone
	// hunting a GPS fault on a board that has no GPS.
	in := sharingNode()
	in.GPS = &bramble.GPSPosition{Valid: false}
	in.Config.Lat = floatPtr(51.5)
	in.Config.Lon = floatPtr(-0.1)

	r := diagnoseLocation(in)
	if !strings.Contains(r.SelfPosition, "manual coordinates") {
		t.Errorf("self position = %q, want it to name the manual coordinates", r.SelfPosition)
	}
	if r.Verdict != verdictOK {
		t.Errorf("verdict = %s, want ok (findings: %v)", r.Verdict, r.Findings)
	}
}

func TestDiagnoseLocationGPSFixWinsOverManualCoordinates(t *testing.T) {
	// Mirrors the firmware's own order: live GPS first, manual as the fallback.
	in := sharingNode()
	in.Config.Lat = floatPtr(51.5)
	in.Config.Lon = floatPtr(-0.1)

	r := diagnoseLocation(in)
	if r.SelfPosition != "GPS fix" {
		t.Errorf("self position = %q, want %q", r.SelfPosition, "GPS fix")
	}
}

func TestDiagnoseLocationChannelTargetNeedsNoSession(t *testing.T) {
	// A channel share is a broadcast under the channel key. Requiring a session
	// for it would report a perfectly working configuration as broken.
	in := sharingNode()
	in.Config.ContactRules = nil
	in.Config.ChannelTargets = []bramble.LocationChannelTarget{{Channel: 0, Tier: "full"}}
	in.Sessions = nil

	r := diagnoseLocation(in)
	tgt := findTarget(t, r, "channel 0")
	if tgt.Verdict != verdictOK {
		t.Errorf("channel target verdict = %s, want ok (%s)", tgt.Verdict, tgt.Reason)
	}
	if !strings.Contains(tgt.Reason, "no session needed") {
		t.Errorf("reason %q does not say a channel share needs no session", tgt.Reason)
	}
	if r.Verdict != verdictOK {
		t.Errorf("verdict = %s, want ok (findings: %v)", r.Verdict, r.Findings)
	}
}

func TestDiagnoseLocationChannelTargetWithNoNeighborsWarnsButDoesNotBlock(t *testing.T) {
	// Transmitting to nobody is a different problem from not transmitting, and
	// conflating them sends someone to reconfigure a node that is fine.
	in := sharingNode()
	in.Config.ContactRules = nil
	in.Config.ChannelTargets = []bramble.LocationChannelTarget{{Channel: 0}}
	in.Neighbors = nil

	r := diagnoseLocation(in)
	tgt := findTarget(t, r, "channel 0")
	if tgt.Verdict != verdictWarn {
		t.Errorf("channel target verdict = %s, want %s (%s)", tgt.Verdict, verdictWarn, tgt.Reason)
	}
	if r.Verdict == verdictBlock {
		t.Errorf("verdict = %s, want it not to be blocked: the node is still transmitting", r.Verdict)
	}
}

func TestDiagnoseLocationDisabledRuleIsNotCountedUsable(t *testing.T) {
	in := sharingNode()
	in.Config.ContactRules = []bramble.LocationContactRule{
		{Address: "DEADBEEF", Enabled: boolPtr(false)},
	}

	r := diagnoseLocation(in)
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s: the only target is switched off", r.Verdict, verdictBlock)
	}
	if got := findTarget(t, r, "DEADBEEF"); got.Enabled {
		t.Error("a disabled rule was reported as enabled")
	}
}

func TestDiagnoseLocationMissingSessionSupportWarnsRatherThanMisreporting(t *testing.T) {
	// Firmware without getDmSessions must not make every contact look broken.
	// Saying "could not check" is honest; saying "no session" would not be.
	in := sharingNode()
	in.Sessions = nil
	in.SessErr = errors.New("method not found")

	r := diagnoseLocation(in)
	tgt := findTarget(t, r, "DEADBEEF")
	if tgt.Verdict != verdictWarn {
		t.Errorf("target verdict = %s, want %s (%s)", tgt.Verdict, verdictWarn, tgt.Reason)
	}
	if !strings.Contains(strings.Join(r.Findings, " "), "session state is unavailable") {
		t.Errorf("findings %v do not say session state could not be read", r.Findings)
	}
}

func TestDiagnoseLocationReportsBothTargetKindsTogether(t *testing.T) {
	in := sharingNode()
	in.Config.ChannelTargets = []bramble.LocationChannelTarget{{Channel: 2}}

	r := diagnoseLocation(in)
	if len(r.Targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(r.Targets))
	}
	findTarget(t, r, "DEADBEEF")
	findTarget(t, r, "channel 2")
}

func TestDiagnoseLocationNoConfigIsBlocked(t *testing.T) {
	r := diagnoseLocation(locationDoctorInput{})
	if r.Verdict != verdictBlock {
		t.Errorf("verdict = %s, want %s", r.Verdict, verdictBlock)
	}
}

func TestWorstVerdictOrdering(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{verdictOK, verdictWarn, verdictWarn},
		{verdictWarn, verdictOK, verdictWarn},
		{verdictWarn, verdictBlock, verdictBlock},
		{verdictBlock, verdictWarn, verdictBlock},
		{verdictOK, verdictOK, verdictOK},
	}
	for _, tc := range cases {
		if got := worst(tc.a, tc.b); got != tc.want {
			t.Errorf("worst(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDiagnoseLocationActiveSessionWithNoRouteWarns(t *testing.T) {
	// A session is necessary but not sufficient: a unicast still needs a
	// neighbor or a route. This is a warning, not a blocker, because a route is
	// built on demand and a neighbor can come back into range on its own.
	in := sharingNode()
	in.Neighbors = nil
	in.Routes = nil

	r := diagnoseLocation(in)
	tgt := findTarget(t, r, "DEADBEEF")
	if tgt.Verdict != verdictWarn {
		t.Errorf("target verdict = %s, want %s (%s)", tgt.Verdict, verdictWarn, tgt.Reason)
	}
	if !strings.Contains(tgt.Reason, "no route") {
		t.Errorf("reason %q does not name the missing route", tgt.Reason)
	}
	if r.Verdict == verdictBlock {
		t.Error("a routable-later peer blocked the whole report")
	}
}

func TestDiagnoseLocationRouteSatisfiesReachabilityWithoutNeighbor(t *testing.T) {
	// A multi-hop peer is reachable without being a direct neighbor.
	in := sharingNode()
	in.Neighbors = nil
	in.Routes = []bramble.Route{{Dest: "DEADBEEF", State: "active", HopCount: 2}}

	r := diagnoseLocation(in)
	if got := findTarget(t, r, "DEADBEEF"); got.Verdict != verdictOK {
		t.Errorf("target verdict = %s, want ok (%s)", got.Verdict, got.Reason)
	}
}

func TestDiagnoseLocationBrokenRouteIsNotReachability(t *testing.T) {
	in := sharingNode()
	in.Neighbors = nil
	in.Routes = []bramble.Route{{Dest: "DEADBEEF", State: "broken"}}

	r := diagnoseLocation(in)
	if got := findTarget(t, r, "DEADBEEF"); got.Verdict != verdictWarn {
		t.Errorf("target verdict = %s, want %s (%s)", got.Verdict, verdictWarn, got.Reason)
	}
}
