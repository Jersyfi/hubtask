// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
)

func TestEachMonitoringProgramsFormatBecomesAlerts(t *testing.T) {
	cases := []struct {
		kind   cmdb.ConnectorKind
		body   string
		key    string
		status cmdb.AlertStatus
		host   string
		title  string
	}{
		{cmdb.ConnectorAlertmanager, `{"version":"4","status":"firing","alerts":[{"status":"firing","fingerprint":"f1",
			"labels":{"alertname":"DiskFull","instance":"web01:9100","severity":"critical"},"annotations":{"summary":"disk full"},
			"startsAt":"2026-10-29T10:00:00Z"}]}`, "f1", cmdb.AlertFiring, "web01:9100", "disk full"},
		{cmdb.ConnectorAlertmanager, `{"alerts":[{"status":"resolved","labels":{"alertname":"Down","instance":"db1"}}]}`,
			"Down/db1", cmdb.AlertResolved, "db1", "Down"},
		{cmdb.ConnectorZabbix, `{"event_id":"77","status":"PROBLEM","severity":"High","host":"web01","title":"CPU high"}`,
			"77", cmdb.AlertFiring, "web01", "CPU high"},
		{cmdb.ConnectorZabbix, `{"event_id":"77","status":"0","host":"web01"}`, "77", cmdb.AlertResolved, "web01", ""},
		{cmdb.ConnectorCheckmk, `{"host":"sw1","service":"Interface 1","state":"CRITICAL","notification_type":"PROBLEM","output":"link down"}`,
			"sw1/Interface 1", cmdb.AlertFiring, "sw1", "link down"},
		{cmdb.ConnectorCheckmk, `{"host":"sw1","state":"UP","notification_type":"RECOVERY"}`, "sw1", cmdb.AlertResolved, "sw1", ""},
	}
	for _, c := range cases {
		got, err := ParseAlerts(c.kind, []byte(c.body))
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: %v %v", c.kind, got, err)
		}
		a := got[0]
		if a.Key != c.key || a.Status != c.status || a.Hosts[0] != c.host || a.Title != c.title {
			t.Errorf("%s: got %+v", c.kind, a)
		}
	}
	if _, err := ParseAlerts(cmdb.ConnectorZabbix, []byte(`not json`)); err == nil {
		t.Error("an unreadable delivery is refused")
	}
}
