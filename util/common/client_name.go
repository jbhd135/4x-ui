package common

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
)

var datedClientCode = regexp.MustCompile(`^(?:\d{4}|\d{6}|\d{8})-([a-zA-Z0-9]{6,12})$`)

// ClientCode is a public lookup label, never a connection credential. Keep this
// in sync with clientLookupCode in web/assets/js/model/inbound.js.
func ClientCode(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	if parts := datedClientCode.FindStringSubmatch(email); len(parts) == 2 {
		return strings.ToLower(parts[1])
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(email))
	return fmt.Sprintf("c%08x", hash.Sum32())
}

func IsDatedClientName(email string) bool {
	return datedClientCode.MatchString(strings.TrimSpace(email))
}

// CheckClientCodeConflicts permits an unchanged email when editing but rejects
// two different customer names that would export the same lookup code.
func CheckClientCodeConflicts(existing, candidates []string) error {
	owners := make(map[string]string, len(existing)+len(candidates))
	for _, email := range existing {
		owners[ClientCode(email)] = strings.TrimSpace(email)
	}
	for _, email := range candidates {
		email = strings.TrimSpace(email)
		code := ClientCode(email)
		if code == "" {
			continue
		}
		if owner, exists := owners[code]; exists && !strings.EqualFold(owner, email) {
			return fmt.Errorf("client lookup code %s is already used; choose a different client name", code)
		}
		owners[code] = email
	}
	return nil
}
