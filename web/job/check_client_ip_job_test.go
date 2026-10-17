package job

import (
	"github.com/mhsanaei/3x-ui/v2/xray"
	"reflect"
	"testing"
	"time"
)

var testAccount = clientIPKey{"inbound-test", "customer-a"}
var testStart = time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))

func observe(key clientIPKey, now time.Time, ips ...string) []clientIPObservation {
	out := make([]clientIPObservation, 0, len(ips))
	for _, ip := range ips {
		out = append(out, clientIPObservation{key, ip, now})
	}
	return out
}

func continuouslyOverLimit(j *CheckClientIpJob) []xray.ClientIPBlock {
	limits := map[clientIPKey]int{testAccount: 1}
	j.evaluate(limits, observe(testAccount, testStart, "192.0.2.1"), testStart)
	var blocks []xray.ClientIPBlock
	for elapsed := 10; elapsed <= 100; elapsed += 10 {
		now := testStart.Add(time.Duration(elapsed) * time.Second)
		blocks = j.evaluate(limits, observe(testAccount, now, "192.0.2.1", "192.0.2.2"), now)
	}
	return blocks
}

func TestClientIPGraceAndRecovery(t *testing.T) {
	j := NewCheckClientIpJob(nil)
	limits := map[clientIPKey]int{testAccount: 1}
	j.evaluate(limits, observe(testAccount, testStart, "192.0.2.1"), testStart)
	for elapsed := 10; elapsed <= 110; elapsed += 10 {
		now := testStart.Add(time.Duration(elapsed) * time.Second)
		got := j.evaluate(limits, observe(testAccount, now, "192.0.2.1", "192.0.2.2"), now)
		if elapsed < 100 && len(got) != 0 {
			t.Fatalf("blocked inside grace at %ds: %v", elapsed, got)
		}
		if elapsed >= 100 && !reflect.DeepEqual(got, []xray.ClientIPBlock{{Inbound: testAccount.Inbound, Email: testAccount.Email, IPs: []string{"192.0.2.2"}}}) {
			t.Fatalf("wrong restriction: %v", got)
		}
	}
	// Incumbent leaves. The previously restricted IP inherits the slot.
	for elapsed := 120; elapsed <= 150; elapsed += 10 {
		now := testStart.Add(time.Duration(elapsed) * time.Second)
		got := j.evaluate(limits, observe(testAccount, now, "192.0.2.2"), now)
		if elapsed == 150 && len(got) != 0 {
			t.Fatalf("restriction did not recover: %v", got)
		}
	}
}

func TestClientIPNetworkSwitchNeverBlocked(t *testing.T) {
	j := NewCheckClientIpJob(nil)
	limits := map[clientIPKey]int{testAccount: 1}
	j.evaluate(limits, observe(testAccount, testStart, "192.0.2.1"), testStart)
	for elapsed := 10; elapsed <= 180; elapsed += 10 {
		now := testStart.Add(time.Duration(elapsed) * time.Second)
		if got := j.evaluate(limits, observe(testAccount, now, "192.0.2.2"), now); len(got) != 0 {
			t.Fatalf("switch blocked: %v", got)
		}
	}
}

func TestClientIPQuietRecoveryAndConfigurationChanges(t *testing.T) {
	for _, mode := range []string{"quiet", "unlimited", "deleted", "limit-increased"} {
		t.Run(mode, func(t *testing.T) {
			j := NewCheckClientIpJob(nil)
			if len(continuouslyOverLimit(j)) != 1 {
				t.Fatal("precondition: expected restriction")
			}
			limits := map[clientIPKey]int{testAccount: 1}
			now := testStart.Add(110 * time.Second)
			obs := observe(testAccount, now, "192.0.2.1", "192.0.2.2")
			switch mode {
			case "quiet":
				now = testStart.Add(131 * time.Second)
				obs = nil
			case "unlimited":
				limits[testAccount] = 0
			case "deleted":
				delete(limits, testAccount)
			case "limit-increased":
				limits[testAccount] = 2
			}
			if got := j.evaluate(limits, obs, now); len(got) != 0 {
				t.Fatalf("not restored: %v", got)
			}
		})
	}
}

func TestClientIPIsolationNormalizationAndIndependentGrace(t *testing.T) {
	j := NewCheckClientIpJob(nil)
	continuouslyOverLimit(j)
	other := clientIPKey{testAccount.Inbound, "customer-b"}
	otherInbound := clientIPKey{"inbound-other", testAccount.Email}
	limits := map[clientIPKey]int{testAccount: 1, other: 1, otherInbound: 1}
	now := testStart.Add(110 * time.Second)
	obs := observe(testAccount, now, "192.0.2.1", "::ffff:192.0.2.1", "192.0.2.2", "2001:db8::3")
	obs = append(obs, observe(other, now, "192.0.2.2")...)
	obs = append(obs, observe(otherInbound, now, "192.0.2.2")...)
	got := j.evaluate(limits, obs, now)
	if len(got) != 1 || got[0].Email != testAccount.Email || got[0].Inbound != testAccount.Inbound || !reflect.DeepEqual(got[0].IPs, []string{"192.0.2.2"}) {
		t.Fatalf("cross-account ban or new IP lost grace: %v", got)
	}
	if len(j.active[testAccount]) != 3 {
		t.Fatal("mapped IPv4 counted twice")
	}
}

func TestClientIPActivityAcrossPollsAndStaleHistory(t *testing.T) {
	j := NewCheckClientIpJob(nil)
	limits := map[clientIPKey]int{testAccount: 1}
	for elapsed := 0; elapsed <= 120; elapsed += 10 {
		now := testStart.Add(time.Duration(elapsed) * time.Second)
		ip := "192.0.2.1"
		if elapsed%20 != 0 {
			ip = "192.0.2.2"
		}
		got := j.evaluate(limits, observe(testAccount, now, ip), now)
		if elapsed == 120 && len(got) != 1 {
			t.Fatal("alternating scans must retain both active sources")
		}
	}
	j = NewCheckClientIpJob(nil)
	got := j.evaluate(limits, []clientIPObservation{
		{testAccount, "192.0.2.1", testStart.Add(-time.Minute)},
		{testAccount, "192.0.2.2", testStart},
		{testAccount, "192.0.2.3", testStart.Add(time.Hour)},
	}, testStart)
	if len(got) != 0 || len(j.active[testAccount]) != 1 {
		t.Fatal("stale/future observations used as admission state")
	}
}

func TestClientIPParseAndHistory(t *testing.T) {
	if got := parseClientAccessTimestamp("2026/10/17 12:00:00", testStart); got != testStart.Unix() {
		t.Fatalf("timezone: %d", got)
	}
	for _, bad := range []string{"bad", "2026/10/17 20:00:00"} {
		if parseClientAccessTimestamp(bad, testStart) != 0 {
			t.Fatal("accepted invalid/future timestamp")
		}
	}
	got := mergeClientIps([]IPWithTimestamp{{"192.0.2.1", 100}, {"192.0.2.2", 1500}}, []IPWithTimestamp{{"192.0.2.2", 2000}, {"192.0.2.3", 500}}, 1000)
	if !reflect.DeepEqual(got, map[string]int64{"192.0.2.2": 2000}) {
		t.Fatalf("history merge: %v", got)
	}
}
