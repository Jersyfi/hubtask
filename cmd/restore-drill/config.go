// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// config is everything the drill is told. It is HUBTASK_* only, like every other entry point
// (arc42 §7.4), and it fails closed: a drill missing its object store would otherwise "restore"
// nothing and report a pass for it.
type config struct {
	// OwnerDSN reaches the live database as its owner: the marker table is the owner's alone.
	OwnerDSN string
	// AppDSN reaches it as hubtask_app, whose bounds the isolation checks are about. Both DSNs are
	// re-pointed at the temporary cluster for the checks, so the same roles and the same
	// passwords - restored with the data - are what the restored instance is judged by.
	AppDSN string

	Namespace     string
	SourceCluster string
	Archive       archiveConfig
	Temporary     temporaryConfig

	// RecordConfigMap is where the result lands: the gauge A-20 reads is fed from it
	// (HUBTASK_RESTORE_DRILL_RECORD_FILE), and the evidence lands in it too when no object store
	// is named.
	RecordConfigMap string
	Evidence        evidenceConfig

	Release  string
	Timeouts timeouts
	// MarkerRetention is how long marker rows stay in the live table. The table is meant to hold
	// a few dozen rows, and P-5's 35 days is the natural bound.
	MarkerRetention time.Duration
	// KeepOnFailure leaves the temporary cluster behind when a check fails, for somebody to look
	// at. Off by default, because a cluster left behind is quota the next drill does not have.
	KeepOnFailure bool
	// FailRelease decides whether a drill that *ran* and found something exits non-zero.
	//
	// The drill is a release hook, and a hook that fails fails the sync - both Helm and Argo CD
	// work that way. That is the wrong outcome for this particular hook: a release whose recovery
	// proof failed is a page, not a release that should not have happened, and blocking deploys
	// on a broken archive also blocks the deploy that would fix it. So by default the outcome is
	// carried by the record and the alerts (A-20, and the never-ran ticket) rather than by an exit
	// code, and the log line says `result=fail` at error level either way.
	//
	// CI sets it, because a build wants the exit code; an installation that would rather stop a
	// release than proceed unproven can set it too. It never covers a *configuration* error: a
	// drill that could not start is a deployment defect, exits non-zero whatever this says, and
	// has no record to carry anything.
	FailRelease bool

	Kube kubeConfig
}

// archiveConfig is the object store the live cluster archives into, exactly as its backup stanza
// names it - the temporary cluster reads from the same place under the same name.
type archiveConfig struct {
	DestinationPath string
	EndpointURL     string
	ServerName      string
	Secret          string
	Compression     string
}

// temporaryConfig shapes the cluster the drill bootstraps. Same image and storage as the live one
// by default, so that what is restored is judged on the same engine.
type temporaryConfig struct {
	Name         string
	ImageName    string
	StorageSize  string
	StorageClass string
	// ResourcesJSON is the live cluster's resources stanza, verbatim: the chart renders it as
	// JSON and the drill embeds it without interpreting it.
	ResourcesJSON string
	DatabaseName  string
	Owner         string
	// OwnerSecret is the Secret the recovered owner's password is set from. Naming the live
	// cluster's own app secret means the live DSNs work against the temporary cluster once their
	// host is swapped - which is the whole trick that lets the checks run under the real roles.
	OwnerSecret string
}

// evidenceConfig is the operator-provided location for what the drill measured. S3-compatible,
// and treated as opaque: a bucket and a prefix the operator chose, and nothing here knows why.
type evidenceConfig struct {
	Endpoint     string
	Bucket       string
	Prefix       string
	Region       string
	AccessKey    secret.Secret
	SecretKey    secret.Secret
	UsePathStyle bool
}

func (e evidenceConfig) configured() bool { return e.Bucket != "" }

type timeouts struct {
	// Total bounds the whole run, teardown included.
	Total time.Duration
	// BackupWait is how long a fresh installation may take to produce its first base backup: a
	// drill needs a recoverability point before it can recover to a moment after it.
	BackupWait time.Duration
	// ArchiveWait bounds how long the archive may lag the second marker's WAL segment. It is the
	// upper bound of the RPO sample, and a wait that expires is the drill's first finding.
	ArchiveWait time.Duration
	// Restore bounds the temporary cluster's bootstrap: the RTO sample's ceiling.
	Restore time.Duration
	// Connect bounds one connection attempt against either database.
	Connect time.Duration
}

type kubeConfig struct {
	// APIURL overrides the in-cluster address (KUBERNETES_SERVICE_HOST/PORT); tests point it at
	// their own server.
	APIURL        string
	TokenFile     string
	CAFile        string
	NamespaceFile string
}

const (
	serviceAccountDir    = "/var/run/secrets/kubernetes.io/serviceaccount"
	defaultTokenFile     = serviceAccountDir + "/token"
	defaultCAFile        = serviceAccountDir + "/ca.crt"
	defaultNamespaceFile = serviceAccountDir + "/namespace"
)

// loadConfig reads the environment through the two functions it is handed, so that a test can
// hand it a map and a fake file system rather than the process's.
func loadConfig(get func(string) string, readFile func(string) ([]byte, error)) (config, error) {
	var problems []string
	require := func(name string) string {
		value := get(name)
		if value == "" {
			problems = append(problems, name+" is required")
		}
		return value
	}
	withDefault := func(name, fallback string) string {
		if value := get(name); value != "" {
			return value
		}
		return fallback
	}
	duration := func(name string, fallback time.Duration) time.Duration {
		raw := get(name)
		if raw == "" {
			return fallback
		}
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			problems = append(problems, name+" is not a positive duration")
			return fallback
		}
		return parsed
	}
	boolean := func(name string) bool {
		raw := get(name)
		if raw == "" {
			return false
		}
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			problems = append(problems, name+" is not a boolean")
		}
		return parsed
	}
	// A secret may arrive as a file, the same convention as every other entry point, so that a
	// Kubernetes secret can be mounted rather than exported.
	secretValue := func(name string) string {
		if value := get(name); value != "" {
			return value
		}
		if path := get(name + "_FILE"); path != "" {
			content, err := readFile(path)
			if err != nil {
				problems = append(problems, name+"_FILE points to a file that cannot be read")
				return ""
			}
			return strings.TrimSpace(string(content))
		}
		return ""
	}

	source := require("HUBTASK_DRILL_SOURCE_CLUSTER")
	cfg := config{
		OwnerDSN:      secretValue("HUBTASK_DB_DSN"),
		AppDSN:        secretValue("HUBTASK_DRILL_APP_DSN"),
		Namespace:     get("HUBTASK_DRILL_NAMESPACE"),
		SourceCluster: source,
		Archive: archiveConfig{
			DestinationPath: require("HUBTASK_DRILL_ARCHIVE_DESTINATION_PATH"),
			EndpointURL:     get("HUBTASK_DRILL_ARCHIVE_ENDPOINT_URL"),
			ServerName:      withDefault("HUBTASK_DRILL_ARCHIVE_SERVER_NAME", source),
			Secret:          require("HUBTASK_DRILL_ARCHIVE_SECRET"),
			Compression:     withDefault("HUBTASK_DRILL_ARCHIVE_COMPRESSION", "gzip"),
		},
		Temporary: temporaryConfig{
			Name:          withDefault("HUBTASK_DRILL_TEMPORARY_CLUSTER", source+"-drill"),
			ImageName:     require("HUBTASK_DRILL_IMAGE_NAME"),
			StorageSize:   require("HUBTASK_DRILL_STORAGE_SIZE"),
			StorageClass:  get("HUBTASK_DRILL_STORAGE_CLASS"),
			ResourcesJSON: get("HUBTASK_DRILL_RESOURCES"),
			DatabaseName:  withDefault("HUBTASK_DRILL_DATABASE_NAME", "hubtask"),
			Owner:         withDefault("HUBTASK_DRILL_OWNER", "hubtask_owner"),
			OwnerSecret:   withDefault("HUBTASK_DRILL_OWNER_SECRET", source+"-app"),
		},
		RecordConfigMap: require("HUBTASK_DRILL_RECORD_CONFIGMAP"),
		Evidence: evidenceConfig{
			Endpoint:     get("HUBTASK_DRILL_EVIDENCE_ENDPOINT"),
			Bucket:       get("HUBTASK_DRILL_EVIDENCE_BUCKET"),
			Prefix:       strings.Trim(get("HUBTASK_DRILL_EVIDENCE_PREFIX"), "/"),
			Region:       withDefault("HUBTASK_DRILL_EVIDENCE_REGION", "us-east-1"),
			AccessKey:    secret.New(secretValue("HUBTASK_DRILL_EVIDENCE_ACCESS_KEY")),
			SecretKey:    secret.New(secretValue("HUBTASK_DRILL_EVIDENCE_SECRET_KEY")),
			UsePathStyle: withDefault("HUBTASK_DRILL_EVIDENCE_USE_PATH_STYLE", "true") == "true",
		},
		Release: get("HUBTASK_DRILL_RELEASE"),
		Timeouts: timeouts{
			Total:       duration("HUBTASK_DRILL_TIMEOUT", 45*time.Minute),
			BackupWait:  duration("HUBTASK_DRILL_BACKUP_WAIT", 15*time.Minute),
			ArchiveWait: duration("HUBTASK_DRILL_ARCHIVE_WAIT", 15*time.Minute),
			Restore:     duration("HUBTASK_DRILL_RESTORE_TIMEOUT", 30*time.Minute),
			Connect:     duration("HUBTASK_DRILL_CONNECT_TIMEOUT", 3*time.Minute),
		},
		MarkerRetention: duration("HUBTASK_DRILL_MARKER_RETENTION", 35*24*time.Hour),
		KeepOnFailure:   boolean("HUBTASK_DRILL_KEEP_ON_FAILURE"),
		FailRelease:     boolean("HUBTASK_DRILL_FAIL_RELEASE"),
		Kube: kubeConfig{
			APIURL:        get("HUBTASK_DRILL_KUBE_API"),
			TokenFile:     withDefault("HUBTASK_DRILL_KUBE_TOKEN_FILE", defaultTokenFile),
			CAFile:        withDefault("HUBTASK_DRILL_KUBE_CA_FILE", defaultCAFile),
			NamespaceFile: withDefault("HUBTASK_DRILL_KUBE_NAMESPACE_FILE", defaultNamespaceFile),
		},
	}
	if cfg.OwnerDSN == "" {
		problems = append(problems, "HUBTASK_DB_DSN is required (the owner's DSN)")
	}
	if cfg.AppDSN == "" {
		problems = append(problems, "HUBTASK_DRILL_APP_DSN is required (the application role's DSN)")
	}
	if cfg.Evidence.configured() && (cfg.Evidence.AccessKey.IsEmpty() || cfg.Evidence.SecretKey.IsEmpty()) {
		problems = append(problems, "HUBTASK_DRILL_EVIDENCE_BUCKET needs HUBTASK_DRILL_EVIDENCE_ACCESS_KEY and _SECRET_KEY")
	}
	if cfg.Namespace == "" {
		content, err := readFile(cfg.Kube.NamespaceFile)
		if err != nil || strings.TrimSpace(string(content)) == "" {
			problems = append(problems, "HUBTASK_DRILL_NAMESPACE is required outside a pod")
		} else {
			cfg.Namespace = strings.TrimSpace(string(content))
		}
	}
	if cfg.Temporary.Name == cfg.SourceCluster {
		problems = append(problems, "HUBTASK_DRILL_TEMPORARY_CLUSTER must differ from the source cluster")
	}
	if len(problems) > 0 {
		return config{}, errors.New(strings.Join(problems, "; "))
	}
	return cfg, nil
}

// evidenceKey is the object the evidence of one run is written under.
func (e evidenceConfig) evidenceKey(runID string) string {
	if e.Prefix == "" {
		return fmt.Sprintf("restore-drill/%s.json", runID)
	}
	return fmt.Sprintf("%s/restore-drill/%s.json", e.Prefix, runID)
}
