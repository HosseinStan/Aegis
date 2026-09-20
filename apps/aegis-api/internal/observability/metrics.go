package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	Requests          atomic.Uint64
	Failures          atomic.Uint64
	DeviceAttempts    atomic.Uint64
	DeviceFailures    atomic.Uint64
	BackupJobs        atomic.Uint64
	BackupFailures    atomic.Uint64
	CacheHits         atomic.Uint64
	CacheMisses       atomic.Uint64
	ActiveConnections atomic.Int64
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "http_requests_total %d\nhttp_request_failures_total %d\ndevice_connection_attempts_total %d\ndevice_connection_failures_total %d\nbackup_jobs_total %d\nbackup_job_failures_total %d\ncache_hits_total %d\ncache_misses_total %d\nactive_device_connections %d\n", m.Requests.Load(), m.Failures.Load(), m.DeviceAttempts.Load(), m.DeviceFailures.Load(), m.BackupJobs.Load(), m.BackupFailures.Load(), m.CacheHits.Load(), m.CacheMisses.Load(), m.ActiveConnections.Load())
}
