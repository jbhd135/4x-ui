package service

import (
	"bytes"
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// SyncClientIPBlocks serializes routing updates with Xray start/stop. The base
// configuration is immutable; temporary restrictions never enter the database.
func (s *XrayService) SyncClientIPBlocks(blocks []xray.ClientIPBlock) error {
	lock.Lock()
	defer lock.Unlock()
	if p == nil || !p.IsRunning() {
		return fmt.Errorf("Xray is not running")
	}
	base := p.GetConfig().RouterConfig
	routes, err := xray.BuildClientIPRoutes(base, blocks)
	if err != nil {
		return err
	}
	if s.clientIPProcess == p && bytes.Equal(routes, s.clientIPRoutes) {
		return nil
	}
	if s.clientIPProcess != p && len(blocks) == 0 {
		s.clientIPProcess, s.clientIPRoutes = p, routes
		return nil
	}
	api := xray.CoreRoutingAPI{Binary: xray.GetBinaryPath(), Port: p.GetAPIPort()}
	applied, err := replaceClientIPRouting(&api, base, routes)
	// Retain process identity even on uncertain failure, so empty desired
	// rules retry rather than being mistaken for a clean process start.
	s.clientIPProcess, s.clientIPRoutes = p, applied
	if err != nil {
		return err
	}
	logger.Infof("[ClientDeviceLimit] Active restricted accounts: %d (account/source scoped)", len(blocks))
	return nil
}

type clientIPRoutingAPI interface{ ReplaceRouting([]byte) error }

func replaceClientIPRouting(api clientIPRoutingAPI, base, routes []byte) ([]byte, error) {
	if err := api.ReplaceRouting(routes); err != nil {
		// A failed replacement may have applied only part of the rules. Restore
		// ordinary routing, not an old restriction that should have expired.
		restore, buildErr := xray.BuildClientIPRoutes(base, nil)
		if buildErr != nil {
			return nil, fmt.Errorf("routing update: %v; rollback config: %w", err, buildErr)
		}
		if restoreErr := api.ReplaceRouting(restore); restoreErr != nil {
			return nil, fmt.Errorf("routing update: %v; rollback failed: %w", err, restoreErr)
		}
		return restore, fmt.Errorf("routing update failed (base routing restored): %w", err)
	}
	return routes, nil
}
