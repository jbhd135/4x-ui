package job

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

type IPWithTimestamp struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

const ipStaleAfterSeconds = int64(30 * 60) // Display history, never admission state.
const clientIPActiveWindow = 30 * time.Second
const clientIPSwitchGrace = 90 * time.Second

type clientIPRouteSyncer interface {
	SyncClientIPBlocks([]xray.ClientIPBlock) error
}
type clientIPKey struct{ Inbound, Email string }
type clientIPObservation struct {
	Key  clientIPKey
	IP   string
	Seen time.Time
}
type clientIPActivity struct{ First, Last, ExcessSince time.Time }

type CheckClientIpJob struct {
	runLock      sync.Mutex
	routes       clientIPRouteSyncer
	active       map[clientIPKey]map[string]*clientIPActivity
	lastPosition int64
	lastFile     os.FileInfo
	lastClear    time.Time
}

func NewCheckClientIpJob(routes clientIPRouteSyncer) *CheckClientIpJob {
	return &CheckClientIpJob{routes: routes, active: make(map[clientIPKey]map[string]*clientIPActivity)}
}

func (j *CheckClientIpJob) Run() {
	if !j.runLock.TryLock() {
		return
	}
	defer j.runLock.Unlock()
	now := time.Now()
	limits, err := loadClientIPLimits()
	if err != nil {
		logger.Warning("[ClientDeviceLimit] Read accounts: ", err)
		if syncErr := j.routes.SyncClientIPBlocks(nil); syncErr != nil {
			logger.Warning("[ClientDeviceLimit] Release restrictions after database error: ", syncErr)
		}
		j.active = make(map[clientIPKey]map[string]*clientIPActivity)
		return
	}
	path, pathErr := xray.GetAccessLogPath()
	var observations []clientIPObservation
	if pathErr == nil && path != "" && path != "none" {
		observations, err = j.readObservations(path, now)
		if err != nil {
			logger.Warning("[ClientDeviceLimit] Read access log: ", err)
		}
	} else if len(limits) > 0 {
		logger.Warning("[ClientDeviceLimit] Access log unavailable; stale restrictions will expire")
	}
	if err := j.saveHistory(observations, now); err != nil {
		logger.Warning("[ClientDeviceLimit] Save history: ", err)
	}
	// Always sweep, including empty/error scans, unlimited and deleted accounts.
	blocks := j.evaluate(limits, observations, now)
	if err := j.routes.SyncClientIPBlocks(blocks); err != nil {
		logger.Warning("[ClientDeviceLimit] Sync restrictions: ", err)
	}
	if j.lastClear.IsZero() {
		j.lastClear = now
	}
	if err == nil && pathErr == nil && path != "" && path != "none" && now.Sub(j.lastClear) > time.Hour {
		if err := j.archiveAccessLog(path); err != nil {
			logger.Warning("[ClientDeviceLimit] Archive access log: ", err)
		} else {
			j.lastClear = now
		}
	}
}

func loadClientIPLimits() (map[clientIPKey]int, error) {
	var inbounds []model.Inbound
	if err := database.GetDB().Where("enable = ?", true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	limits := make(map[clientIPKey]int)
	for _, inbound := range inbounds {
		var settings struct {
			Clients []struct {
				Email   string `json:"email"`
				Enable  *bool  `json:"enable"`
				LimitIP int    `json:"limitIp"`
			} `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return nil, fmt.Errorf("inbound %d settings: %w", inbound.Id, err)
		}
		for _, c := range settings.Clients {
			if (c.Enable == nil || *c.Enable) && c.Email != "" && c.LimitIP > 0 {
				limits[clientIPKey{inbound.Tag, c.Email}] = c.LimitIP
			}
		}
	}
	return limits, nil
}

// Access-log activity approximates simultaneous public IPs, not physical devices.
// Stable first-seen ordering keeps an incumbent's slot while a newcomer gets
// its own grace period. No user enable flag or established session is changed.
func (j *CheckClientIpJob) evaluate(limits map[clientIPKey]int, observations []clientIPObservation, now time.Time) []xray.ClientIPBlock {
	if j.active == nil {
		j.active = make(map[clientIPKey]map[string]*clientIPActivity)
	}
	cutoff := now.Add(-clientIPActiveWindow)
	for key, ips := range j.active {
		if limits[key] <= 0 {
			delete(j.active, key)
			continue
		}
		for ip, activity := range ips {
			if activity.Last.Before(cutoff) {
				delete(ips, ip)
			}
		}
	}
	for _, o := range observations {
		if limits[o.Key] <= 0 || o.Seen.Before(cutoff) || o.Seen.After(now.Add(time.Second)) {
			continue
		}
		addr, err := netip.ParseAddr(o.IP)
		if err != nil || addr.Zone() != "" {
			continue
		}
		ip := addr.Unmap().String()
		if j.active[o.Key] == nil {
			j.active[o.Key] = make(map[string]*clientIPActivity)
		}
		activity := j.active[o.Key][ip]
		if activity == nil {
			activity = &clientIPActivity{First: o.Seen, Last: o.Seen}
			j.active[o.Key][ip] = activity
		} else if o.Seen.After(activity.Last) {
			activity.Last = o.Seen
		}
	}
	var blocks []xray.ClientIPBlock
	for key, ips := range j.active {
		if len(ips) == 0 {
			delete(j.active, key)
			continue
		}
		ordered := make([]string, 0, len(ips))
		for ip := range ips {
			ordered = append(ordered, ip)
		}
		sort.Slice(ordered, func(i, j int) bool {
			a, b := ips[ordered[i]], ips[ordered[j]]
			if !a.First.Equal(b.First) {
				return a.First.Before(b.First)
			}
			return ordered[i] < ordered[j]
		})
		block := xray.ClientIPBlock{Inbound: key.Inbound, Email: key.Email}
		for index, ip := range ordered {
			a := ips[ip]
			if index < limits[key] {
				a.ExcessSince = time.Time{}
				continue
			}
			if a.ExcessSince.IsZero() {
				a.ExcessSince = now
			}
			if now.Sub(a.ExcessSince) >= clientIPSwitchGrace {
				block.IPs = append(block.IPs, ip)
			}
		}
		if len(block.IPs) > 0 {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

var clientAccessPattern = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)? .*from (?:tcp:|udp:)?\[?([0-9a-fA-F:.]+)\]?:\d+ accepted .*\[([^\s\]]+)\s+(?:->|>>)\s+[^\]]+\].*email: (.+)$`)

func parseClientAccessTimestamp(value string, now time.Time) int64 {
	parsed, err := time.ParseInLocation("2006/01/02 15:04:05", value, now.Location())
	if err != nil || parsed.After(now.Add(time.Second)) {
		return 0
	}
	return parsed.Unix()
}

func (j *CheckClientIpJob) readObservations(path string, now time.Time) ([]clientIPObservation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if j.lastFile == nil || !os.SameFile(j.lastFile, info) || j.lastPosition > info.Size() {
		j.lastPosition = 0
	}
	j.lastFile = info
	if _, err := f.Seek(j.lastPosition, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(f)
	type observationKey struct {
		Account clientIPKey
		IP      string
	}
	latest := map[observationKey]clientIPObservation{}
	var readErr error
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break // Retry an incomplete line next scan.
		}
		j.lastPosition += int64(len(line))
		m := clientAccessPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 5 {
			continue
		}
		addr, err := netip.ParseAddr(m[2])
		if err != nil || addr.Unmap().IsLoopback() {
			continue
		}
		ts := parseClientAccessTimestamp(m[1], now)
		if ts == 0 || ts < now.Add(-clientIPActiveWindow).Unix() {
			continue
		}
		key := observationKey{clientIPKey{m[3], m[4]}, addr.Unmap().String()}
		seen := time.Unix(ts, 0)
		if old, ok := latest[key]; !ok || seen.After(old.Seen) {
			latest[key] = clientIPObservation{key.Account, key.IP, seen}
		}
	}
	observations := make([]clientIPObservation, 0, len(latest))
	for _, o := range latest {
		observations = append(observations, o)
	}
	return observations, readErr
}

func mergeClientIps(old, fresh []IPWithTimestamp, cutoff int64) map[string]int64 {
	merged := map[string]int64{}
	for _, entries := range [][]IPWithTimestamp{old, fresh} {
		for _, entry := range entries {
			if entry.Timestamp >= cutoff && entry.Timestamp > merged[entry.IP] {
				merged[entry.IP] = entry.Timestamp
			}
		}
	}
	return merged
}

func (j *CheckClientIpJob) saveHistory(observations []clientIPObservation, now time.Time) error {
	byEmail := map[string][]IPWithTimestamp{}
	for _, o := range observations {
		byEmail[o.Key.Email] = append(byEmail[o.Key.Email], IPWithTimestamp{o.IP, o.Seen.Unix()})
	}
	for email, fresh := range byEmail {
		row := model.InboundClientIps{ClientEmail: email}
		err := database.GetDB().Where("client_email = ?", email).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var old []IPWithTimestamp
		_ = json.Unmarshal([]byte(row.Ips), &old)
		merged := mergeClientIps(old, fresh, now.Unix()-ipStaleAfterSeconds)
		entries := make([]IPWithTimestamp, 0, len(merged))
		for ip, ts := range merged {
			entries = append(entries, IPWithTimestamp{ip, ts})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].IP < entries[j].IP })
		encoded, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		row.Ips = string(encoded)
		if err := database.GetDB().Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (j *CheckClientIpJob) archiveAccessLog(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if j.lastFile == nil || !os.SameFile(j.lastFile, info) || info.Size() != j.lastPosition {
		return nil
	}
	dst, err := os.OpenFile(xray.GetAccessPersistentLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.CopyN(dst, f, info.Size()); err != nil {
		return err
	}
	current, err := f.Stat()
	if err != nil {
		return err
	}
	if current.Size() != info.Size() {
		return nil
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	j.lastPosition = 0
	return nil
}
