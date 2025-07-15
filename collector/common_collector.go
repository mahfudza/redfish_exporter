package collector

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish/redfish"
)

const (
	CommonStateHelp           = "1(Enabled),2(Disabled),3(StandbyOffinline),4(StandbySpare),5(InTest),6(Starting),7(Absent),8(UnavailableOffline),9(Deferring),10(Quiesced),11(Updating)"
	CommonHealthHelp          = "1(OK),2(Warning),3(Critical)"
	CommonSeverityHelp        = CommonHealthHelp
	CommonLinkHelp            = "1(LinkUp),2(NoLink),3(LinkDown)"
	CommonPortLinkHelp        = "1(Up),0(Down)"
	CommonIntrusionSensorHelp = "1(Normal),2(TamperingDetected),3(HardwareIntrusion)"
	CommonChargeStateHelp     = "1(Idle),2(Charging),3(Discharging)"
)

type Metric struct {
	desc *prometheus.Desc
}

func addToMetricMap(metricMap map[string]Metric, subsystem, name, help string, variableLabels []string) {
	metricKey := fmt.Sprintf("%s_%s", subsystem, name)
	metricMap[metricKey] = Metric{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, name),
			help,
			variableLabels,
			nil,
		),
	}
}

func parseCommonChargeState(chargeState redfish.ChargeState) (float64, bool) {
	switch chargeState {
	case redfish.IdleChargeState:
		return float64(1), true
	case redfish.ChargingChargeState:
		return float64(2), true
	case redfish.DischargingChargeState:
		return float64(3), true
	}
	return float64(0), false
}

func parseLogService(ch chan<- prometheus.Metric, metrics map[string]Metric, subsystem, collectorID string, logService *redfish.LogService, wg *sync.WaitGroup) (err error) {
	defer wg.Done()
	logServiceName := logService.Name
	logServiceID := logService.ID
	logServiceEnabled := fmt.Sprint(logService.ServiceEnabled)
	logServiceOverWritePolicy := string(logService.OverWritePolicy)
	logServiceState := logService.Status.State
	logServiceHealthState := logService.Status.Health

	logServiceLabelValues := []string{collectorID, logServiceName, logServiceID, logServiceEnabled, logServiceOverWritePolicy}

	if logServiceStateValue, ok := parseCommonStatusState(logServiceState); ok {
		ch <- prometheus.MustNewConstMetric(metrics[fmt.Sprintf("%s_%s", subsystem, "log_service_state")].desc, prometheus.GaugeValue, logServiceStateValue, logServiceLabelValues...)
	}
	if logServiceHealthStateValue, ok := parseCommonStatusHealth(logServiceHealthState); ok {
		ch <- prometheus.MustNewConstMetric(metrics[fmt.Sprintf("%s_%s", subsystem, "log_service_health_state")].desc, prometheus.GaugeValue, logServiceHealthStateValue, logServiceLabelValues...)
	}

	return
}
