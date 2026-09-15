package job

import "sync"

// Serializes jobs that read and rewrite inbound client settings. Several of
// these jobs run at midnight, so allowing them to overlap can re-save stale
// settings after the daily-limit reset has re-enabled a client.
var inboundMaintenanceLock sync.Mutex
