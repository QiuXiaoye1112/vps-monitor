package report

// WithTrafficPersistencePaused prevents the minute persistence worker from
// crossing a strict manual-clear boundary. Live reports may continue arriving
// while fn waits for the Agent snapshot; after fn commits the new baseline,
// all buffered pre-boundary reports for that node are discarded. The next
// report is then measured from the persisted snapshot counters.
func WithTrafficPersistencePaused(uuid string, fn func() error) error {
	saveClientReportMu.Lock()
	defer saveClientReportMu.Unlock()

	if err := fn(); err != nil {
		return err
	}

	reportCacheMu.Lock()
	Records.Delete(uuid)
	reportCacheMu.Unlock()
	return nil
}
