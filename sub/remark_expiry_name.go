package sub

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/common"
)

var clientRemarkDateTokens = regexp.MustCompile(`\d+`)
var datedClientIdentifier = regexp.MustCompile(`^(?:\d{4}|\d{6}|\d{8})-(.+)$`)

func (s *SubService) clientLookupRemark(inbound *model.Inbound, email, extra string) string {
	label := "高速"
	clients, _ := s.inboundService.GetClients(inbound)
	var expiry int64
	found := false
	for _, client := range clients {
		if client.Email == email {
			expiry, found = client.ExpiryTime, true
			if client.SocksProxyEnabled {
				label = "家宽"
			}
			break
		}
	}
	if name := strings.TrimSpace(email); name != "" && !common.IsDatedClientName(name) {
		label = name
	}
	parts := []string{label}
	if found {
		switch {
		case expiry > 0:
			parts = append(parts, time.UnixMilli(expiry).In(s.remarkDateNow().Location()).Format("06-01-02"))
		case expiry < 0:
			parts = append(parts, fmt.Sprintf("启用后%d天", -expiry/86400000))
		case strings.TrimSpace(email) == "" || common.IsDatedClientName(email):
			parts = append(parts, "不限期")
		}
	} else if date, ok := parseClientRemarkExpiryDate(inbound.Remark, s.remarkDateNow()); ok {
		parts = append(parts, date.Format("06-01-02"))
	}
	remark := strings.Join(parts, "| ")
	if code := common.ClientCode(email); code != "" {
		remark = code + "|" + remark
	}
	if extra = strings.TrimSpace(extra); extra != "" {
		remark += " | " + extra
	}
	return remark
}

func (s *SubService) clientVisibleInboundRemark(remark string) (string, bool) {
	if formatted, ok := formatClientExpiryRemark(remark, s.remarkDateNow()); ok {
		return formatted, true
	}
	return remark, false
}

func (s *SubService) remarkDateNow() (now time.Time) {
	now = time.Now()
	defer func() {
		if recover() != nil {
			now = time.Now()
		}
	}()
	if loc, err := s.settingService.GetTimeLocation(); err == nil && loc != nil {
		return now.In(loc)
	}
	return now
}

func formatClientExpiryRemark(remark string, now time.Time) (string, bool) {
	expiry, ok := parseClientRemarkExpiryDate(remark, now)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("过期日期:%d年%d月%d日", expiry.Year(), int(expiry.Month()), expiry.Day()), true
}

func formatClientIdentifierSuffix(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	if matches := datedClientIdentifier.FindStringSubmatch(email); len(matches) == 2 {
		return matches[1]
	}
	return email
}

func parseClientRemarkExpiryDate(remark string, now time.Time) (time.Time, bool) {
	remark = strings.TrimSpace(remark)
	if remark == "" {
		return time.Time{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}

	var earliest time.Time
	for _, token := range clientRemarkDateTokens.FindAllString(remark, -1) {
		date, ok := parseClientRemarkDateToken(token, now)
		if !ok {
			continue
		}
		if earliest.IsZero() || date.Before(earliest) {
			earliest = date
		}
	}
	if earliest.IsZero() {
		return time.Time{}, false
	}
	return earliest, true
}

func parseClientRemarkDateToken(token string, now time.Time) (time.Time, bool) {
	year := now.Year()
	mmdd := token
	switch len(token) {
	case 4:
	case 6:
		parsedYear, err := strconv.Atoi(token[:2])
		if err != nil {
			return time.Time{}, false
		}
		year = 2000 + parsedYear
		mmdd = token[2:]
	case 8:
		parsedYear, err := strconv.Atoi(token[:4])
		if err != nil || parsedYear < 2000 || parsedYear > 2099 {
			return time.Time{}, false
		}
		year = parsedYear
		mmdd = token[4:]
	default:
		return time.Time{}, false
	}

	month, err := strconv.Atoi(mmdd[:2])
	if err != nil || month < 1 || month > 12 {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(mmdd[2:])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}

	loc := now.Location()
	if loc == nil {
		loc = time.Local
	}
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc), true
}
