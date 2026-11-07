// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AlertDeliverer is the application's side of an alert intake.
type AlertDeliverer interface {
	DeliverAlerts(ctx context.Context, d cmdbservice.AlertDelivery, parse cmdbservice.AlertParser) (cmdbservice.AlertReceipt, error)
}

// AlertIntake translates a monitoring program's delivery into alerts (research itsm-cmdb-ai
// C10): an Alertmanager webhook, a Zabbix media type's post, a Checkmk notification. What each
// alert means - which CI, which entry - is decided inwards; this only reads the program's format.
type AlertIntake struct {
	Deliveries AlertDeliverer
}

// Deliver hands the delivery and its credential on, with the parser of the intake's kind.
func (i AlertIntake) Deliver(ctx context.Context, d cmdbservice.AlertDelivery) (cmdbservice.AlertReceipt, error) {
	return i.Deliveries.DeliverAlerts(ctx, d, ParseAlerts)
}

func malformed() error { return shared.ErrMalformedRequest.WithDetail("cmdb.alerts_malformed") }

// ParseAlerts reads a delivery in its kind's format.
func ParseAlerts(kind cmdb.ConnectorKind, body []byte) ([]cmdb.AlertSignal, error) {
	switch kind {
	case cmdb.ConnectorAlertmanager:
		return alertmanager(body)
	case cmdb.ConnectorZabbix:
		return zabbix(body)
	case cmdb.ConnectorCheckmk:
		return checkmk(body)
	}
	return nil, malformed()
}

func decodeInto(body []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(into); err != nil {
		return malformed()
	}
	return nil
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	}
	return ""
}

func when(v string) *time.Time {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil || t.IsZero() || t.Year() < 2000 {
		return nil
	}
	return &t
}

// alertmanager reads Prometheus Alertmanager's webhook (version 4): each alert by its
// fingerprint, its instance and host labels naming the CI.
func alertmanager(body []byte) ([]cmdb.AlertSignal, error) {
	var doc struct {
		Alerts []struct {
			Status      string            `json:"status"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			StartsAt    string            `json:"startsAt"`
			Fingerprint string            `json:"fingerprint"`
		} `json:"alerts"`
	}
	if err := decodeInto(body, &doc); err != nil {
		return nil, err
	}
	out := make([]cmdb.AlertSignal, 0, len(doc.Alerts))
	for _, a := range doc.Alerts {
		key := a.Fingerprint
		if key == "" {
			key = a.Labels["alertname"] + "/" + a.Labels["instance"]
		}
		status := cmdb.AlertFiring
		if strings.EqualFold(a.Status, "resolved") {
			status = cmdb.AlertResolved
		}
		title := a.Annotations["summary"]
		if title == "" {
			title = a.Labels["alertname"]
		}
		out = append(out, cmdb.AlertSignal{
			Key: key, Status: status, Severity: a.Labels["severity"], Title: title, At: when(a.StartsAt),
			Hosts: []string{a.Labels["instance"], a.Labels["host"], a.Labels["hostname"], a.Labels["nodename"]},
		})
	}
	return out, nil
}

// zabbix reads the post of a Zabbix webhook media type whose parameters are named as
// automation.md §6 shows: one problem by its event, PROBLEM or 1 firing, RESOLVED,
// OK or 0 resolved.
func zabbix(body []byte) ([]cmdb.AlertSignal, error) {
	var doc map[string]any
	if err := decodeInto(body, &doc); err != nil {
		return nil, err
	}
	key := str(doc["event_id"])
	if key == "" {
		key = str(doc["trigger_id"])
	}
	status := cmdb.AlertFiring
	switch strings.ToUpper(str(doc["status"])) {
	case "RESOLVED", "OK", "0":
		status = cmdb.AlertResolved
	}
	return []cmdb.AlertSignal{{
		Key: key, Status: status, Severity: str(doc["severity"]), Title: str(doc["title"]),
		Hosts: []string{str(doc["host"]), str(doc["host_name"]), str(doc["host_ip"])},
	}}, nil
}

// checkmk reads a Checkmk notification posted as JSON: a host or one of its services by name,
// a recovery or an OK or UP state resolving it.
func checkmk(body []byte) ([]cmdb.AlertSignal, error) {
	var doc map[string]any
	if err := decodeInto(body, &doc); err != nil {
		return nil, err
	}
	host, service := str(doc["host"]), str(doc["service"])
	key := host
	if service != "" {
		key = host + "/" + service
	}
	state := strings.ToUpper(str(doc["state"]))
	status := cmdb.AlertFiring
	if strings.EqualFold(str(doc["notification_type"]), "RECOVERY") || state == "OK" || state == "UP" {
		status = cmdb.AlertResolved
	}
	title := str(doc["output"])
	if title == "" {
		title = service
	}
	return []cmdb.AlertSignal{{
		Key: key, Status: status, Severity: state, Title: title, Hosts: []string{host, str(doc["address"])},
	}}, nil
}
