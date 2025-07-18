package collector

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	gofish "github.com/stmcginnis/gofish"
	gofishcommon "github.com/stmcginnis/gofish/common"
	redfish "github.com/stmcginnis/gofish/redfish"
)

// Metric name parts.
const (
	namespace = "redfish"
	exporter  = "exporter"
)

var (
	totalScrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, exporter, "collector_duration_seconds"),
		"Collector time duration.",
		nil, nil,
	)
)

// RedfishCollector collects redfish metrics. It implements prometheus.Collector.
type RedfishCollector struct {
	redfishClient *gofish.APIClient
	collectors    map[string]prometheus.Collector
	redfishUp     prometheus.Gauge
}

// NewRedfishCollector return RedfishCollector
func NewRedfishCollector(host string, username string, password string) *RedfishCollector {
	var collectors map[string]prometheus.Collector

	targetLogger := slog.Default().With(slog.String("target", host))

	redfishClient, err := newRedfishClient(host, username, password)
	if err != nil {
		slog.Error("error creating redfish client", slog.Any("error", err))
	} else {
		chassisCollector := NewChassisCollector(redfishClient, targetLogger)
		systemCollector := NewSystemCollector(redfishClient, targetLogger)
		managerCollector := NewManagerCollector(redfishClient, targetLogger)
		firmwareCollector := NewFirmwareCollector(redfishClient, targetLogger)

		collectors = map[string]prometheus.Collector{
			"chassis":  chassisCollector,
			"system":   systemCollector,
			"manager":  managerCollector,
			"firmware": firmwareCollector,
		}
	}

	return &RedfishCollector{
		redfishClient: redfishClient,
		collectors:    collectors,
		redfishUp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "",
				Name:      "up",
				Help:      "redfish up",
			},
		),
	}
}

// Describe implements prometheus.Collector.
func (r *RedfishCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, collector := range r.collectors {
		collector.Describe(ch)
	}

}

// Collect implements prometheus.Collector.
func (r *RedfishCollector) Collect(ch chan<- prometheus.Metric) {

	scrapeTime := time.Now()
	if r.redfishClient != nil {
		defer r.redfishClient.Logout()
		r.redfishUp.Set(1)
		wg := &sync.WaitGroup{}
		wg.Add(len(r.collectors))

		for _, collector := range r.collectors {
			go func(collector prometheus.Collector) {
				defer wg.Done()
				collector.Collect(ch)
			}(collector)
		}
		wg.Wait()
	} else {
		r.redfishUp.Set(0)
	}

	ch <- r.redfishUp
	ch <- prometheus.MustNewConstMetric(totalScrapeDurationDesc, prometheus.GaugeValue, time.Since(scrapeTime).Seconds())
}

func newRedfishClient(host string, username string, password string) (*gofish.APIClient, error) {

	url := fmt.Sprintf("https://%s", host)

	config := gofish.ClientConfig{
		Endpoint: url,
		Username: username,
		Password: password,
		Insecure: true,
	}
	redfishClient, err := gofish.Connect(config)
	if err != nil {
		return nil, err
	}
	return redfishClient, nil
}

func parseCommonStatusHealth(status gofishcommon.Health) (float64, bool) {
	if bytes.Equal([]byte(status), []byte("OK")) {
		return float64(1), true
	} else if bytes.Equal([]byte(status), []byte("Warning")) {
		return float64(2), true
	} else if bytes.Equal([]byte(status), []byte("Critical")) {
		return float64(3), true
	}
	return float64(0), false
}

func parseCommonStatusState(status gofishcommon.State) (float64, bool) {

	if bytes.Equal([]byte(status), []byte("")) {
		return float64(0), false
	} else if bytes.Equal([]byte(status), []byte("Enabled")) {
		return float64(1), true
	} else if bytes.Equal([]byte(status), []byte("Disabled")) {
		return float64(2), true
	} else if bytes.Equal([]byte(status), []byte("StandbyOffinline")) {
		return float64(3), true
	} else if bytes.Equal([]byte(status), []byte("StandbySpare")) {
		return float64(4), true
	} else if bytes.Equal([]byte(status), []byte("InTest")) {
		return float64(5), true
	} else if bytes.Equal([]byte(status), []byte("Starting")) {
		return float64(6), true
	} else if bytes.Equal([]byte(status), []byte("Absent")) {
		return float64(7), true
	} else if bytes.Equal([]byte(status), []byte("UnavailableOffline")) {
		return float64(8), true
	} else if bytes.Equal([]byte(status), []byte("Deferring")) {
		return float64(9), true
	} else if bytes.Equal([]byte(status), []byte("Quiesced")) {
		return float64(10), true
	} else if bytes.Equal([]byte(status), []byte("Updating")) {
		return float64(11), true
	}
	return float64(0), false
}

func parseCommonPowerState(status redfish.PowerState) (float64, bool) {
	if bytes.Equal([]byte(status), []byte("On")) {
		return float64(1), true
	} else if bytes.Equal([]byte(status), []byte("Off")) {
		return float64(2), true
	} else if bytes.Equal([]byte(status), []byte("PoweringOn")) {
		return float64(3), true
	} else if bytes.Equal([]byte(status), []byte("PoweringOff")) {
		return float64(4), true
	}
	return float64(0), false
}

func parseLinkStatus(status redfish.LinkStatus) (float64, bool) {
	if bytes.Equal([]byte(status), []byte("LinkUp")) {
		return float64(1), true
	} else if bytes.Equal([]byte(status), []byte("NoLink")) {
		return float64(2), true
	} else if bytes.Equal([]byte(status), []byte("LinkDown")) {
		return float64(3), true
	}
	return float64(0), false
}

func parsePortLinkStatus(status redfish.PortLinkStatus) (float64, bool) {
	if bytes.Equal([]byte(status), []byte("Up")) {
		return float64(1), true
	}
	return float64(0), false
}

func parseNetworkPortLinkStatus(status redfish.NetworkPortLinkStatus) (float64, bool) {
	if bytes.Equal([]byte(status), []byte("Up")) {
		return float64(1), true
	}
	return float64(0), false
}
func boolToFloat64(data bool) float64 {

	if data {
		return float64(1)
	}
	return float64(0)

}

func parsePhySecIntrusionSensor(method redfish.IntrusionSensor) (float64, bool) {
	if bytes.Equal([]byte(method), []byte("Normal")) {
		return float64(1), true
	}
	if bytes.Equal([]byte(method), []byte("TamperingDetected")) {
		return float64(2), true
	}
	if bytes.Equal([]byte(method), []byte("HardwareIntrusion")) {
		return float64(3), true
	}

	return float64(0), false
}
