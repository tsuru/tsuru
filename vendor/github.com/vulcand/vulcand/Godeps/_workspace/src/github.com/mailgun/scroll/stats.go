package scroll

import (
	"fmt"
	"net/http"
	"time"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/metrics"
)

type appStats struct {
	c metrics.Client
}

func newAppStats(client metrics.Client) *appStats {
	return &appStats{
		c: client,
	}
}

func (s *appStats) TrackRequest(metricID string, status int, time time.Duration) {
	if s.c == nil {
		return
	}

	s.TrackRequestTime(metricID, time)
	s.TrackTotalRequests(metricID)
	if status != http.StatusOK {
		s.TrackFailedRequests(metricID, status)
	}
}

func (s *appStats) TrackRequestTime(metricID string, time time.Duration) {
	s.c.TimingMs(fmt.Sprintf("api.%v.time", metricID), time, 1.0)
}

func (s *appStats) TrackTotalRequests(metricID string) {
	s.c.Inc(fmt.Sprintf("api.%v.count.total", metricID), 1, 1.0)
}

func (s *appStats) TrackFailedRequests(metricID string, status int) {
	s.c.Inc(fmt.Sprintf("api.%v.count.failed.%v", metricID, status), 1, 1.0)
}
