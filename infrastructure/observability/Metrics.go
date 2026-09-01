// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// namespace prefixes every metric, so that a dashboard can select the project's own series
// without knowing each name (observability-reliability.md §4).
const namespace = "hubtask"

// Metrics owns the meter and the instruments that exist before any use case does. Handing out an
// instrument rather than the meter is deliberate: an instrument created at a call site is created
// per call, and the label set of one created ad hoc is nobody's decision.
type Metrics struct {
	registry *prometheus.Registry
	provider *sdkmetric.MeterProvider

	panicsRecovered   metric.Int64Counter
	httpRequests      metric.Int64Counter
	httpDuration      metric.Float64Histogram
	inflightRequests  metric.Int64UpDownCounter
	useCaseTotal      metric.Int64Counter
	dependencyUp      metric.Int64Gauge
	degradedMode      metric.Int64Gauge
	configInvalid     metric.Int64Counter
	breakerState      metric.Int64Gauge
	outboundDuration  metric.Float64Histogram
	rateLimited       metric.Int64Counter
	jobDuration       metric.Float64Histogram
	jobFailures       metric.Int64Counter
	jobDeadLetters    metric.Int64Counter
	jobQueueDepth     metric.Int64Gauge
	quotaUsageRatio   metric.Float64Gauge
	outboxLag         metric.Float64Histogram
	schedulerTickLag  metric.Int64Gauge
	backupLastSuccess metric.Int64Gauge
	dsrDeadline       metric.Int64Counter
	retentionDeleted  metric.Int64Counter
	retentionBlocked  metric.Int64Counter
	retentionRun      metric.Float64Histogram
	mediaReclaimed    metric.Int64Counter
	mediaReclaimFail  metric.Int64Counter
	notificationsRec  metric.Int64Counter
	notificationsSent metric.Float64Histogram
	notificationsFail metric.Int64Counter
	reminderDelay     metric.Float64Histogram
	occurrenceLag     metric.Float64Histogram
	streamsOpen       metric.Int64UpDownCounter
	streamDuration    metric.Float64Histogram
	streamRefused     metric.Int64Counter
	streamRecords     metric.Int64Counter
	authFailures      metric.Int64Counter
	poolConnections   metric.Int64Gauge
	migrationVersion  metric.Int64Gauge
	ruleRuns          metric.Int64Counter
	ruleDisabled      metric.Int64Counter
	tenantLabelActive bool
}

// NewMetrics builds the meter and registers what the process needs from the first second: the
// panic counter above all, whose value is the one an alert watches (ADR-0016).
func NewMetrics(cfg env.Config) (*Metrics, error) {
	registry := prometheus.NewRegistry()

	exporter, err := otelprom.New(
		otelprom.WithRegisterer(registry),
		// The runtime already reports its own version through build_info; the exporter's
		// duplicate of it is noise on every series.
		otelprom.WithoutScopeInfo(),
		otelprom.WithoutTargetInfo(),
	)
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter(namespace)

	m := &Metrics{
		registry:          registry,
		provider:          provider,
		tenantLabelActive: cfg.Metrics.TenantLabel,
	}

	if err := m.instruments(meter); err != nil {
		return nil, err
	}

	// A counter that has never counted has no series, and a series that does not exist cannot
	// be alerted on - `absent()` fires, or the dashboard shows "no data" and everyone assumes
	// the panel is broken. Seeding zero makes the metric readable from the first scrape, which
	// is what "exists and stays at 0" asks for (ADR-0016).
	m.PanicRecoveredBy(context.Background(), "process", 0)
	if err := m.buildInfo(meter, cfg); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Metrics) instruments(meter metric.Meter) error {
	var err error
	if m.panicsRecovered, err = meter.Int64Counter(
		namespace+"_panics_recovered_total",
		metric.WithDescription("Panics caught by SafeGo or a recovery middleware. Target value: 0."),
	); err != nil {
		return fmt.Errorf("panic counter: %w", err)
	}
	if m.httpRequests, err = meter.Int64Counter(
		namespace+"_http_requests_total",
		metric.WithDescription("HTTP requests by route, method and status class."),
	); err != nil {
		return fmt.Errorf("request counter: %w", err)
	}
	if m.httpDuration, err = meter.Float64Histogram(
		namespace+"_http_request_duration_seconds",
		metric.WithDescription("HTTP request duration."),
		metric.WithUnit("s"),
		// Buckets around the targets from engineering-guidelines.md §4: P95 read < 200 ms,
		// write < 300 ms. Default buckets would put both in the same bucket.
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2.5, 5, 10),
	); err != nil {
		return fmt.Errorf("duration histogram: %w", err)
	}
	if m.inflightRequests, err = meter.Int64UpDownCounter(
		namespace+"_inflight_requests",
		metric.WithDescription("Requests currently being served, by role."),
	); err != nil {
		return fmt.Errorf("inflight gauge: %w", err)
	}
	if m.useCaseTotal, err = meter.Int64Counter(
		namespace+"_usecase_total",
		metric.WithDescription("Use case executions by outcome."),
	); err != nil {
		return fmt.Errorf("use case counter: %w", err)
	}
	if m.dependencyUp, err = meter.Int64Gauge(
		namespace+"_dependency_up",
		metric.WithDescription("Self-diagnosis as a time series: 1 up, 0 down."),
	); err != nil {
		return fmt.Errorf("dependency gauge: %w", err)
	}
	if m.degradedMode, err = meter.Int64Gauge(
		namespace+"_degraded_mode",
		metric.WithDescription("1 while a feature is restricted."),
	); err != nil {
		return fmt.Errorf("degradation gauge: %w", err)
	}
	if m.configInvalid, err = meter.Int64Counter(
		namespace+"_config_invalid_total",
		metric.WithDescription("Configuration the process flagged at startup, by key. A hard "+
			"rejection stops the process before the exporter exists - what this carries are the "+
			"tolerated misconfigurations /meta/health warns about, as a series A-14 can watch."),
	); err != nil {
		return fmt.Errorf("config counter: %w", err)
	}
	if m.poolConnections, err = meter.Int64Gauge(
		namespace+"_db_pool_connections",
		metric.WithDescription("Connections per pool by state: in_use, idle, and the configured "+
			"max, which is what turns the first two into a utilisation (A-11)."),
	); err != nil {
		return fmt.Errorf("pool gauge: %w", err)
	}
	if m.migrationVersion, err = meter.Int64Gauge(
		namespace+"_migration_version",
		metric.WithDescription("The migration this build embeds. Differing values across the "+
			"cluster mean a rollout in progress; differing for long means a stuck one (A-13)."),
	); err != nil {
		return fmt.Errorf("migration gauge: %w", err)
	}
	if m.breakerState, err = meter.Int64Gauge(
		namespace+"_circuit_breaker_state",
		metric.WithDescription("0 closed, 1 half-open, 2 open, per guarded dependency."),
	); err != nil {
		return fmt.Errorf("breaker gauge: %w", err)
	}
	if m.outboundDuration, err = meter.Float64Histogram(
		namespace+"_outbound_http_duration_seconds",
		metric.WithDescription("Duration of outbound HTTP calls by target class."),
		metric.WithUnit("s"),
		// Wider than the inbound buckets: a third-party system that answers in five seconds is
		// unpleasant but real, and the inbound scale would put everything slow in one bucket.
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30),
	); err != nil {
		return fmt.Errorf("outbound histogram: %w", err)
	}
	if m.rateLimited, err = meter.Int64Counter(
		namespace+"_rate_limited_total",
		metric.WithDescription("Calls turned away by a limit: a rate limit, a bulkhead, or load shedding."),
	); err != nil {
		return fmt.Errorf("rate limit counter: %w", err)
	}
	return m.queueInstruments(meter)
}

// queueInstruments are the signals of the work that happens outside a request (ADR-0008,
// ADR-0007). They are separate from the block above only for length; the rules are the same, and
// the label is always the job kind - a closed set written by hand, never an identifier (§3.2).
func (m *Metrics) queueInstruments(meter metric.Meter) error {
	var err error
	if m.jobDuration, err = meter.Float64Histogram(
		namespace+"_job_duration_seconds",
		metric.WithDescription("How long a job took, by kind."),
		metric.WithUnit("s"),
		// Wider than a request: a job may legitimately take a minute, and the inbound scale would
		// put everything that matters into its last bucket.
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300),
	); err != nil {
		return fmt.Errorf("job duration histogram: %w", err)
	}
	if m.jobFailures, err = meter.Int64Counter(
		namespace+"_job_failures_total",
		metric.WithDescription("Failed job attempts, by kind and by whether another attempt follows."),
	); err != nil {
		return fmt.Errorf("job failure counter: %w", err)
	}
	if m.jobDeadLetters, err = meter.Int64Counter(
		namespace+"_job_dead_letter_total",
		metric.WithDescription("Jobs that used up their attempts. Every one of these needs a person."),
	); err != nil {
		return fmt.Errorf("dead letter counter: %w", err)
	}
	if m.jobQueueDepth, err = meter.Int64Gauge(
		namespace+"_job_queue_depth",
		metric.WithDescription("Jobs waiting and due, by kind. Reported by the scheduler leader only."),
	); err != nil {
		return fmt.Errorf("queue depth gauge: %w", err)
	}
	if m.quotaUsageRatio, err = meter.Float64Gauge(
		namespace+"_tenant_quota_usage_ratio",
		metric.WithDescription("How close a workspace stands to one of its §4 ceilings: used/limit, by quota. What A-18 watches."),
	); err != nil {
		return fmt.Errorf("quota usage gauge: %w", err)
	}
	if m.outboxLag, err = meter.Float64Histogram(
		namespace+"_outbox_lag_seconds",
		metric.WithDescription("Age of an event when it reached its consumers. SLO-4."),
		metric.WithUnit("s"),
		// The target is a P99 under thirty seconds, so the buckets are dense below it and coarse
		// above: past a minute the question is no longer how late but why.
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.5, 1, 2, 5, 10, 30, 60, 300),
	); err != nil {
		return fmt.Errorf("outbox lag histogram: %w", err)
	}
	if m.schedulerTickLag, err = meter.Int64Gauge(
		namespace+"_scheduler_tick_lag_seconds",
		metric.WithDescription("How late the scheduler's last tick was against when it was due."),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("tick lag gauge: %w", err)
	}
	if m.backupLastSuccess, err = meter.Int64Gauge(
		namespace+"_backup_last_success_timestamp_seconds",
		metric.WithDescription(
			"When each backup target last had a backup that succeeded, as a Unix timestamp."),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("backup freshness gauge: %w", err)
	}
	if m.dsrDeadline, err = meter.Int64Counter(
		namespace+"_dsr_deadline_total",
		metric.WithDescription(
			"Data subject requests reported at each pass of the deadline watch, by stage."),
	); err != nil {
		return fmt.Errorf("data subject deadline counter: %w", err)
	}
	if m.authFailures, err = meter.Int64Counter(
		namespace+"_auth_failures_total",
		metric.WithDescription("Refused sign-in and refresh attempts, by reason. refresh_reused is A-15's second half: a rotated refresh token presented again means two holders of one credential."),
	); err != nil {
		return fmt.Errorf("auth failure counter: %w", err)
	}
	if m.ruleRuns, err = meter.Int64Counter(
		namespace+"_rule_runs_total",
		metric.WithDescription("Ended automation runs by result and trigger kind. SLO-7's share "+
			"of runs without an internal error divides failed by everything."),
	); err != nil {
		return fmt.Errorf("rule run counter: %w", err)
	}
	if m.ruleDisabled, err = meter.Int64Counter(
		namespace+"_rule_disabled_total",
		metric.WithDescription("Rules that switched themselves off, by reason. What A-16 "+
			"watches: the loop and error protection becoming visible."),
	); err != nil {
		return fmt.Errorf("rule disabled counter: %w", err)
	}
	if err := m.retentionInstruments(meter); err != nil {
		return err
	}
	if err := m.mediaInstruments(meter); err != nil {
		return err
	}
	if err := m.notificationInstruments(meter); err != nil {
		return err
	}
	return m.streamInstruments(meter)
}

// streamInstruments are what the change stream publishes (C-10).
//
// No tenant and no account on any of them. A series per workspace is the cardinality problem
// §3.2 is about, and on a gauge of open connections it would additionally say who is online -
// which is presence data nobody asked to publish (rule 10).
func (m *Metrics) streamInstruments(meter metric.Meter) error {
	var err error
	if m.streamsOpen, err = meter.Int64UpDownCounter(
		namespace+"_stream_connections",
		metric.WithDescription("Change streams this process is currently holding open."),
	); err != nil {
		return fmt.Errorf("stream connection gauge: %w", err)
	}
	if m.streamDuration, err = meter.Float64Histogram(
		namespace+"_stream_duration_seconds",
		metric.WithDescription("How long a change stream stayed open."),
		metric.WithUnit("s"),
		// Minutes and hours rather than milliseconds: a stream that lasted a second is a client
		// reconnecting in a loop, and a stream that lasted a day is one working as intended. Both
		// ends of that range are worth telling apart.
		metric.WithExplicitBucketBoundaries(1, 10, 60, 300, 900, 3600, 14400, 86400),
	); err != nil {
		return fmt.Errorf("stream duration histogram: %w", err)
	}
	if m.streamRefused, err = meter.Int64Counter(
		namespace+"_stream_refused_total",
		metric.WithDescription("Stream connections refused, by which limit refused them."),
	); err != nil {
		return fmt.Errorf("stream refusal counter: %w", err)
	}
	if m.streamRecords, err = meter.Int64Counter(
		namespace+"_stream_records_total",
		metric.WithDescription("Change records delivered over the streams."),
	); err != nil {
		return fmt.Errorf("stream record counter: %w", err)
	}
	return nil
}

// StreamOpened counts a connection this process has taken on.
func (m *Metrics) StreamOpened(ctx context.Context) { m.streamsOpen.Add(ctx, 1) }

// StreamClosed gives the connection back and records how long it lasted.
func (m *Metrics) StreamClosed(ctx context.Context, seconds float64) {
	m.streamsOpen.Add(ctx, -1)
	m.streamDuration.Record(ctx, seconds)
}

// StreamRefused counts a connection that was not taken on, by the limit that refused it. The
// reason is the point: "twelve refused" is not something an operator can act on, and "twelve
// refused by the per-credential cap" is.
func (m *Metrics) StreamRefused(ctx context.Context, reason string) {
	m.streamRefused.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

// StreamRecords counts what went down the streams. Against the change log's own growth it is what
// says whether the streams are keeping up.
func (m *Metrics) StreamRecords(ctx context.Context, count int) {
	m.streamRecords.Add(ctx, int64(count))
}

// notificationInstruments are what the notification path publishes (C-09).
//
// The labels are the closed sets the notification domain defines - the category, the channel, the
// state and the suppression reasons - so nothing unbounded can reach a label from here
// (observability-reliability.md §3.2). No recipient and no tenant: a series per person is the
// cardinality explosion §3.2 is about, and it would also be personal data in a metric (rule 10).
func (m *Metrics) notificationInstruments(meter metric.Meter) error {
	var err error
	if m.notificationsRec, err = meter.Int64Counter(
		namespace+"_notifications_recorded_total",
		metric.WithDescription(
			"Notification records written, by category, channel and the state they were written in."),
	); err != nil {
		return fmt.Errorf("notification counter: %w", err)
	}
	if m.notificationsSent, err = meter.Float64Histogram(
		namespace+"_notification_send_duration_seconds",
		metric.WithDescription("How long sending one notification took, by category and channel."),
		metric.WithUnit("s"),
		// A mail server on the same network answers in milliseconds and one across the internet in
		// seconds. Past thirty the question is no longer how slow but whether it is there at all,
		// which is the breaker's business rather than this histogram's.
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30),
	); err != nil {
		return fmt.Errorf("notification send histogram: %w", err)
	}
	if m.notificationsFail, err = meter.Int64Counter(
		namespace+"_notification_failures_total",
		metric.WithDescription("Notification sends that did not work, by category, channel and reason."),
	); err != nil {
		return fmt.Errorf("notification failure counter: %w", err)
	}
	if m.occurrenceLag, err = meter.Float64Histogram(
		namespace+"_recurrence_occurrence_lag_seconds",
		metric.WithDescription(
			"How late an occurrence was created against the moment it is for, by recurrence mode."),
		metric.WithUnit("s"),
		// A rolling window means most occurrences are created *before* their moment, which is
		// reported as zero. What these boundaries measure is the other direction: a scheduler that
		// stopped and came back. Past a day the question is no longer how late but whether
		// anything is running, which A-06 answers.
		metric.WithExplicitBucketBoundaries(0, 60, 300, 900, 3600, 21600, 86400),
	); err != nil {
		return fmt.Errorf("occurrence lag histogram: %w", err)
	}
	if m.reminderDelay, err = meter.Float64Histogram(
		namespace+"_reminder_delivery_delay_seconds",
		metric.WithDescription(
			"How late a reminder was against the moment it promised, by channel."),
		metric.WithUnit("s"),
		// The boundaries are SLO-5's: the objective is "within 60 s", so the buckets have to be
		// able to answer that question exactly rather than around it - which is why 60 is one of
		// them and 120, alert A-08's threshold, is the next. Past an hour the question is no
		// longer how late but whether the scheduler is running at all, which A-06 answers.
		metric.WithExplicitBucketBoundaries(1, 5, 15, 30, 60, 120, 300, 900, 3600),
	); err != nil {
		return fmt.Errorf("reminder delay histogram: %w", err)
	}
	return nil
}

// NotificationRecorded counts a record as it is written, in the state it was written in. The state
// is the point: a rising SUPPRESSED count is people switching things off, and a rising PENDING
// count that never becomes SENT is a mail server nobody has noticed.
func (m *Metrics) NotificationRecorded(ctx context.Context, category, channel, state string) {
	m.notificationsRec.Add(ctx, 1, metric.WithAttributes(
		attribute.String("category", category),
		attribute.String("channel", channel),
		attribute.String("state", state),
	))
}

// NotificationSent records how long one message took to leave.
func (m *Metrics) NotificationSent(
	ctx context.Context, category, channel string, seconds float64,
) {
	m.notificationsSent.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("category", category),
		attribute.String("channel", channel),
	))
}

// NotificationFailed counts a send that did not work, by the detail code that says why. The reason
// separates "the server is down" from "the address was refused", which are different problems with
// different fixes.
func (m *Metrics) NotificationFailed(ctx context.Context, category, channel, reason string) {
	m.notificationsFail.Add(ctx, 1, metric.WithAttributes(
		attribute.String("category", category),
		attribute.String("channel", channel),
		attribute.String("reason", reason),
	))
}

// ReminderFired records how late one reminder was against the moment it promised (SLO-5, D-03).
//
// The delay rather than the send: a reminder that left on time and then waited an hour for a mail
// server is a different failure, and the notification metrics already measure that one. What this
// answers is whether the schedule kept its promise.
func (m *Metrics) ReminderFired(ctx context.Context, channel string, delaySeconds float64) {
	m.reminderDelay.Record(ctx, delaySeconds, metric.WithAttributes(
		attribute.String("channel", channel),
	))
}

// OccurrenceMaterialized records how late an occurrence was created against the moment it is for
// (D-05, the lag ADR-0008 promised).
//
// The mode is the label because the two answer different questions: an ON_SCHEDULE series that is
// behind is a scheduler problem, while an ON_COMPLETION one is behind only for as long as nobody
// completed anything - which is a person's business rather than an operator's.
func (m *Metrics) OccurrenceMaterialized(ctx context.Context, mode string, lagSeconds float64) {
	m.occurrenceLag.Record(ctx, lagSeconds, metric.WithAttributes(
		attribute.String("mode", mode),
	))
}

// mediaInstruments are the two numbers the media reclamation publishes (C-06,
// data-protection.md §5).
//
// Counters and no labels at all: the question is "is unreferenced storage actually being reclaimed,
// and is anything failing to be", and neither half of it is per anything. A label naming the object
// or the tenant would be exactly the unbounded label observability-reliability.md §3.2 forbids.
func (m *Metrics) mediaInstruments(meter metric.Meter) error {
	var err error
	if m.mediaReclaimed, err = meter.Int64Counter(
		namespace+"_media_reclaimed_total",
		metric.WithDescription("Media objects removed for good because nothing referenced them."),
	); err != nil {
		return fmt.Errorf("media reclamation counter: %w", err)
	}
	if m.mediaReclaimFail, err = meter.Int64Counter(
		namespace+"_media_reclaim_failed_total",
		metric.WithDescription("Orphaned media objects whose bytes storage would not release."),
	); err != nil {
		return fmt.Errorf("media reclamation failure counter: %w", err)
	}
	return nil
}

// MediaReclaimed counts what a pass removed. Written even when it is zero, for the reason
// RetentionDeleted is.
func (m *Metrics) MediaReclaimed(ctx context.Context, count int64) {
	m.mediaReclaimed.Add(ctx, count)
}

// MediaReclaimFailed counts the orphans a pass could not reclaim. They stay marked and the next
// pass tries again, so a number that keeps rising is a bucket that is not letting go rather than a
// backlog that will clear.
func (m *Metrics) MediaReclaimFailed(ctx context.Context, count int64) {
	m.mediaReclaimFail.Add(ctx, count)
}

// retentionInstruments are the three numbers data-retention.md §5 asks a deletion run to publish.
//
// Counters rather than gauges for what was removed and what was kept: the question an operator asks
// is "how much has gone since", not "how much went in the last run", and a gauge answers only the
// second. The labels are the closed sets the lifecycle package defines - one data kind and two block
// reasons - so nothing unbounded can reach a label from here
// (observability-reliability.md §3.2).
func (m *Metrics) retentionInstruments(meter metric.Meter) error {
	var err error
	if m.retentionDeleted, err = meter.Int64Counter(
		namespace+"_retention_deleted_total",
		metric.WithDescription("Rows removed for good by the retention runs, by data kind."),
	); err != nil {
		return fmt.Errorf("retention deletion counter: %w", err)
	}
	if m.retentionBlocked, err = meter.Int64Counter(
		namespace+"_retention_blocked_total",
		metric.WithDescription("Rows past their period that were kept, by reason."),
	); err != nil {
		return fmt.Errorf("retention block counter: %w", err)
	}
	if m.retentionRun, err = meter.Float64Histogram(
		namespace+"_retention_run_duration_seconds",
		metric.WithDescription("How long one pass of a retention run took, by data kind."),
		metric.WithUnit("s"),
		// A pass is one batch, so it should be short. Past thirty seconds the question is no longer
		// how long but what is holding the transaction.
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60),
	); err != nil {
		return fmt.Errorf("retention run histogram: %w", err)
	}
	return nil
}

// AuthFailure counts one refused sign-in or refresh attempt, by reason. The reasons are the
// closed set the identity service declares - never a subject, never an address (§3.2, rule 10).
func (m *Metrics) AuthFailure(ctx context.Context, reason string) {
	m.authFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

// RetentionDeleted counts what a pass removed. Written even when it is zero: a counter that has
// never been written has no series, and an alert on a deletion run that never happens is one that
// reads "no data" and is believed (observability-reliability.md §4).
func (m *Metrics) RetentionDeleted(ctx context.Context, dataKind string, count int64) {
	m.retentionDeleted.Add(ctx, count, metric.WithAttributes(attribute.String("data_kind", dataKind)))
}

// RetentionBlocked counts what a pass kept, by reason. The reason is the point: "twelve were kept"
// is not something an operator can act on, and "twelve were kept by a legal hold" is.
func (m *Metrics) RetentionBlocked(ctx context.Context, reason string, count int64) {
	m.retentionBlocked.Add(ctx, count, metric.WithAttributes(attribute.String("reason", reason)))
}

// RetentionRun records how long one pass took.
func (m *Metrics) RetentionRun(ctx context.Context, dataKind string, seconds float64) {
	m.retentionRun.Record(ctx, seconds, metric.WithAttributes(attribute.String("data_kind", dataKind)))
}

// buildInfo is the gauge that is always 1 and whose labels are the point: it makes the version
// situation across a cluster visible, which is what a rolling update needs.
func (m *Metrics) buildInfo(meter metric.Meter, cfg env.Config) error {
	gauge, err := meter.Int64Gauge(
		namespace+"_build_info",
		metric.WithDescription("Always 1; the labels carry the build."),
	)
	if err != nil {
		return fmt.Errorf("build info: %w", err)
	}
	gauge.Record(context.Background(), 1, metric.WithAttributes(
		attribute.String("version", cfg.Version),
		attribute.String("commit", cfg.Commit),
		attribute.String("go_version", runtime.Version()),
	))
	return nil
}

// Handler serves the Prometheus endpoint. It belongs on the ops port, never on the public one
// (§3.2): the series say what runs, how much of it, and how slowly.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// A scrape failure is a monitoring problem, not an application one: report it to the
		// scraper and keep serving.
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// PanicRecovered counts a caught panic. component is the location, not the message: a panic
// message can contain anything, including user content (rule 10).
func (m *Metrics) PanicRecovered(ctx context.Context, component string) {
	m.PanicRecoveredBy(ctx, component, 1)
}

// PanicRecoveredBy exists for the zero seeding above; a delta of anything but 0 or 1 would be a
// misuse, and the counter is not the place to be clever.
func (m *Metrics) PanicRecoveredBy(ctx context.Context, component string, delta int64) {
	m.panicsRecovered.Add(ctx, delta, metric.WithAttributes(attribute.String("component", component)))
}

// HTTPRequest records one served request. route is the template (/items/{id}), never the
// resolved path - the resolved one carries identifiers and would be unbounded.
func (m *Metrics) HTTPRequest(ctx context.Context, route, method string, status int, seconds float64) {
	attrs := metric.WithAttributes(
		attribute.String("route", route),
		attribute.String("method", method),
		attribute.String("status_class", statusClass(status)),
	)
	m.httpRequests.Add(ctx, 1, attrs)
	m.httpDuration.Record(ctx, seconds,
		metric.WithAttributes(attribute.String("route", route), attribute.String("method", method)))
}

// InflightDelta moves the gauge of requests currently in flight.
func (m *Metrics) InflightDelta(ctx context.Context, role string, delta int64) {
	m.inflightRequests.Add(ctx, delta, metric.WithAttributes(attribute.String("role", role)))
}

// UseCase records the outcome of a use case. The result is the error category, not the message:
// ok, or one of the domain's categories in lower case (§4.1) - see ResultClass, which is the only
// thing that should be producing this value.
func (m *Metrics) UseCase(ctx context.Context, useCase, result string, tenant string) {
	attrs := []attribute.KeyValue{
		attribute.String("use_case", useCase),
		attribute.String("result", result),
	}
	// The tenant label is off unless the operator asks for it: in provider operation with
	// thousands of tenants it multiplies every series by the tenant count (§3.2).
	if m.tenantLabelActive && tenant != "" {
		attrs = append(attrs, attribute.String("tenant_id", tenant))
	}
	m.useCaseTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// QuotaUsage records how close one workspace stands to one of its ceilings (H-08). The label is
// the quota - a bounded set of six - and the tenant joins it only when the operator enabled the
// tenant label (§3.2): without it the series carries the last workspace's number per quota,
// which is still the alarm A-18 needs ("somebody stands at 90%"), just not the who.
func (m *Metrics) QuotaUsage(ctx context.Context, quota string, tenant string, ratio float64) {
	attrs := []attribute.KeyValue{attribute.String("quota", quota)}
	if m.tenantLabelActive && tenant != "" {
		attrs = append(attrs, attribute.String("tenant_id", tenant))
	}
	m.quotaUsageRatio.Record(ctx, ratio, metric.WithAttributes(attrs...))
}

// DependencyUp mirrors the health report into a time series, so that a disruption has a
// beginning and an end in the same place as everything else.
func (m *Metrics) DependencyUp(ctx context.Context, dependency string, up bool) {
	value := int64(0)
	if up {
		value = 1
	}
	m.dependencyUp.Record(ctx, value, metric.WithAttributes(attribute.String("dependency", dependency)))
}

// DegradedMode reports a feature as restricted, which is what makes a partial outage visible
// without reading the health endpoint.
func (m *Metrics) DegradedMode(ctx context.Context, feature string, degraded bool) {
	value := int64(0)
	if degraded {
		value = 1
	}
	m.degradedMode.Record(ctx, value, metric.WithAttributes(attribute.String("feature", feature)))
}

// CircuitBreakerState publishes the state of one breaker: 0 closed, 1 half-open, 2 open. The
// values come from resilience.BreakerState.Level(), which owns them - the gauge only reports
// what it is handed, so that the dashboard's contract has a single source (§4).
//
// The composition root passes this as the breaker's OnStateChange hook. It takes the level
// rather than the state type, so that the metrics adapter does not have to know the resilience
// adapter: adapters do not know each other (project-structure.md §2).
func (m *Metrics) CircuitBreakerState(ctx context.Context, dependency string, level int64) {
	m.breakerState.Record(ctx, level, metric.WithAttributes(attribute.String("dependency", dependency)))
}

// OutboundHTTP records the duration of one outbound call. targetClass is a class, never a host:
// a webhook target is chosen by a tenant, and a label per URL would grow a series per customer
// integration (§3.2, rule 10).
func (m *Metrics) OutboundHTTP(ctx context.Context, targetClass string, seconds float64) {
	m.outboundDuration.Record(ctx, seconds,
		metric.WithAttributes(attribute.String("target_class", targetClass)))
}

// RateLimited counts a call turned away by a limit. scope says which limit: ip, token, tenant
// for the rate limiter, load_shed for the shedder, bulkhead:<compartment> for a full
// compartment. The set is written by hand and stays small - that is what keeps it a label.
func (m *Metrics) RateLimited(ctx context.Context, scope string) {
	m.rateLimited.Add(ctx, 1, metric.WithAttributes(attribute.String("scope", scope)))
}

// JobFinished records a job that ran to the end. kind is the job kind, which is a closed set in
// core/port/queue - never a tenant, never an identifier.
func (m *Metrics) JobFinished(ctx context.Context, kind string, seconds float64) {
	m.jobDuration.Record(ctx, seconds, metric.WithAttributes(attribute.String("job_type", kind)))
}

// JobFailed records one failed attempt. attemptClass is retry or final, which is the distinction
// an alert needs: retries are the system working, and a final failure is a person's problem.
func (m *Metrics) JobFailed(ctx context.Context, kind, attemptClass string) {
	m.jobFailures.Add(ctx, 1, metric.WithAttributes(
		attribute.String("job_type", kind),
		attribute.String("attempt_class", attemptClass),
	))
}

// JobDeadLettered counts a job that used up its attempts (alert A-07).
func (m *Metrics) JobDeadLettered(ctx context.Context, kind string) {
	m.jobDeadLetters.Add(ctx, 1, metric.WithAttributes(attribute.String("job_type", kind)))
}

// QueueDepth publishes the backlog of one kind. Only the scheduler leader reports it: the number
// describes the installation, and one written by every replica would be summed across instances
// and read as several times the truth.
func (m *Metrics) QueueDepth(ctx context.Context, kind string, pending int64) {
	m.jobQueueDepth.Record(ctx, pending, metric.WithAttributes(attribute.String("job_type", kind)))
}

// OutboxLag records the age of an event at delivery, which is what SLO-4 promises and what alert
// A-05 watches. Deliberately without a tenant label even when the tenant label is enabled: it is a
// histogram, and one per tenant multiplies buckets rather than series.
func (m *Metrics) OutboxLag(ctx context.Context, seconds float64) {
	m.outboxLag.Record(ctx, seconds)
}

// SchedulerTickLag publishes how late the last tick was. Seconds as a whole number: the gauge
// exists to show drift, and a schedule that is off by less than a second is not drifting.
func (m *Metrics) SchedulerTickLag(ctx context.Context, seconds float64) {
	m.schedulerTickLag.Record(ctx, int64(seconds))
}

// BackupLastSuccess publishes when a target last had a backup that worked - the number alert A-12
// has been watching since 0.2.0 with nothing behind it (E-05, backup-restore.md §10).
//
// A Unix timestamp rather than an age, which is the convention every backup exporter follows and
// the reason it works: an age computed here is stale the moment it is scraped, while a timestamp
// lets the alert compute the age at evaluation time and stay right between scrapes.
//
// Labelled by target and not by tenant. Which targets exist is bounded by configuration; how many
// tenants there are is not, and an unbounded label is how a metrics backend falls over
// (observability-reliability.md §3.2).
func (m *Metrics) BackupLastSuccess(ctx context.Context, targetID string, at time.Time) {
	m.backupLastSuccess.Record(ctx, at.Unix(),
		metric.WithAttributes(attribute.String("target_id", targetID)))
}

// DataSubjectDeadline reports cases whose statutory deadline is near or past - alert A-19's number
// (E-10, data-protection.md §4).
//
// A counter by stage rather than a gauge of how many are open, and that is about what is true in
// provider operation: a gauge would need a tenant label to be true there, and an unlabelled one
// would carry the last workspace's number rather than the installation's. "A deadline is
// approaching somewhere" is true either way, and it is what the alert asks.
func (m *Metrics) DataSubjectDeadline(ctx context.Context, stage string, count int) {
	m.dsrDeadline.Add(ctx, int64(count),
		metric.WithAttributes(attribute.String("stage", stage)))
}

// ConfigInvalid counts a rejected configuration variable - misconfiguration as a signal rather
// than as guesswork in a support conversation.
func (m *Metrics) ConfigInvalid(ctx context.Context, key string) {
	m.configInvalid.Add(ctx, 1, metric.WithAttributes(attribute.String("key", key)))
}

// RuleRun counts one ended automation run. Both values come from the engine's closed sets - the
// domain's status vocabulary and the six trigger kinds - never a rule or a tenant (§3.2).
func (m *Metrics) RuleRun(ctx context.Context, result, triggerType string) {
	m.ruleRuns.Add(ctx, 1, metric.WithAttributes(
		attribute.String("result", result), attribute.String("trigger_type", triggerType)))
}

// RuleDisabled counts one rule switching itself off. The reason is the engine's, and there is
// exactly one today; the label exists so a second protection would be told apart from the first.
func (m *Metrics) RuleDisabled(ctx context.Context, reason string) {
	m.ruleDisabled.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

// PoolConnections publishes one pool's connection states. The max joins in_use and idle as a
// third state rather than a separate metric, because it is the denominator A-11 divides by and
// belongs on the same series a dashboard already reads. The pool label tells the API's pool from
// the background one - their saturation means different things (§6).
func (m *Metrics) PoolConnections(ctx context.Context, pool string, inUse, idle, maxConns int64) {
	for state, value := range map[string]int64{"in_use": inUse, "idle": idle, "max": maxConns} {
		m.poolConnections.Record(ctx, value, metric.WithAttributes(
			attribute.String("pool", pool), attribute.String("state", state)))
	}
}

// MigrationVersion publishes the migration this build embeds, once at startup. It deliberately
// reports the build, not the database: every pod asks the same database, so the database's answer
// could never differ between pods, and differing is the whole signal (A-13).
func (m *Metrics) MigrationVersion(ctx context.Context, version int64) {
	m.migrationVersion.Record(ctx, version)
}

// Shutdown flushes the meter. Metrics missing from the last seconds of a pod's life are exactly
// the ones an incident review wants.
func (m *Metrics) Shutdown(ctx context.Context) error {
	return m.provider.Shutdown(ctx)
}

// statusClass reduces a status to its class. The exact code would give 5xx a series per code and
// tell an alert nothing more than the class does.
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}
