package collector

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	gofish "github.com/stmcginnis/gofish"
	redfish "github.com/stmcginnis/gofish/redfish"
)

// FirmwareSubsystem is the firmware subsystem
var (
	FirmwareSubsystem  = "firmware"
	FirmwareLabelNames = []string{"name", "version", "updateable"}
	firmwareMetrics    = createFirmwareMetricMap()
)

// FirmwareCollector implements the prometheus.Collector.
type FirmwareCollector struct {
	redfishClient *gofish.APIClient
	metrics       map[string]Metric
	logger        *slog.Logger
	prometheus.Collector
	collectorScrapeStatus *prometheus.GaugeVec
}

func createFirmwareMetricMap() map[string]Metric {
	firmwareMetrics := make(map[string]Metric)

	addToMetricMap(firmwareMetrics, FirmwareSubsystem, "info", "firmware information", FirmwareLabelNames)

	return firmwareMetrics
}

// NewFirmwareCollector returns a collector that collecting firmware statistics
func NewFirmwareCollector(redfishClient *gofish.APIClient, logger *slog.Logger) *FirmwareCollector {
	return &FirmwareCollector{
		redfishClient: redfishClient,
		metrics:       firmwareMetrics,
		logger:        logger,
		collectorScrapeStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "collector_scrape_status",
				Help:      "collector_scrape_status",
			},
			[]string{"collector"},
		),
	}
}

// Describe implements prometheus.Collector.
func (f *FirmwareCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range f.metrics {
		ch <- metric.desc
	}
	f.collectorScrapeStatus.Describe(ch)
}

// Collect implements prometheus.Collector.
func (f *FirmwareCollector) Collect(ch chan<- prometheus.Metric) {
	logger := f.logger.With(slog.String("collector", "FirmwareCollector"))
	service := f.redfishClient.Service

	// get update service
	updateService, err := service.UpdateService()
	if err != nil {
		logger.Error("error getting update service", slog.String("operation", "service.UpdateService()"), slog.Any("error", err))
		return
	}
	logger.Debug("successfully got update service")

	// get firmware inventory
	firmwareInventory, err := updateService.FirmwareInventories()
	if err != nil {
		logger.Error("error getting firmware inventory", slog.String("operation", "updateService.FirmwareInventories()"), slog.Any("error", err))
		return
	}
	logger.Debug("successfully got firmware inventory", slog.Int("count", len(firmwareInventory)))

	if len(firmwareInventory) == 0 {
		logger.Info("no firmware inventory found", slog.String("operation", "updateService.FirmwareInventories()"))
		return
	}

	// Get hostname from first system
	systems, err := f.redfishClient.Service.Systems()
	if err != nil {
		logger.Error("error getting systems", slog.String("operation", "service.Systems()"), slog.Any("error", err))
		return
	}
	if len(systems) == 0 {
		logger.Error("no systems found", slog.String("operation", "service.Systems()"))
		return
	}

	// Use a map to track unique firmware entries and prevent duplicates
	uniqueFirmware := make(map[string]bool)
	var uniqueFirmwareList []*redfish.SoftwareInventory

	// Deduplicate firmware entries
	for _, firmware := range firmwareInventory {
		if firmware == nil {
			logger.Error("nil firmware inventory item found")
			continue
		}

		// Create a unique key based on name and version
		uniqueKey := fmt.Sprintf("%s-%s", firmware.Name, firmware.Version)

		if !uniqueFirmware[uniqueKey] {
			uniqueFirmware[uniqueKey] = true
			uniqueFirmwareList = append(uniqueFirmwareList, firmware)
		} else {
			logger.Debug("skipping duplicate firmware entry",
				slog.String("name", firmware.Name),
				slog.String("version", firmware.Version),
			)
		}
	}

	logger.Debug("deduplicated firmware inventory",
		slog.Int("original_count", len(firmwareInventory)),
		slog.Int("unique_count", len(uniqueFirmwareList)),
	)

	if len(uniqueFirmwareList) == 0 {
		logger.Info("no unique firmware inventory found after deduplication")
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(uniqueFirmwareList))

	for _, firmware := range uniqueFirmwareList {
		go func(firmware *redfish.SoftwareInventory) {
			defer wg.Done()

			firmwareName := firmware.Name
			firmwareVersion := firmware.Version
			firmwareUpdateable := firmware.Updateable

			firmwareLabelValues := []string{
				firmwareName,
				firmwareVersion,
			}
			logger.Debug("collecting firmware metric",
				slog.String("name", firmwareName),
				slog.String("version", firmwareVersion),
				slog.Bool("updateable", firmwareUpdateable),
			)

			ch <- prometheus.MustNewConstMetric(f.metrics["firmware_info"].desc, prometheus.GaugeValue, 1, firmwareLabelValues...)
		}(firmware)
	}

	wg.Wait()
	f.collectorScrapeStatus.WithLabelValues("firmware").Set(float64(1))
}
