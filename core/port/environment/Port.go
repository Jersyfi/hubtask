// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package environment is the port for the entire configuration.
//
// The core never reads from os.Getenv itself. All values come from HUBTASK_* variables
// (12-factor) and are loaded and validated in the adapter. If a required secret is missing,
// the process does not start - fail closed rather than a silent default (ADR-0015).
package environment

import "github.com/Jersyfi/hubtask/core/shared/secret"

// Role denotes a process role. One image, several roles (ADR-0014).
type Role string

const (
	RoleAPI        Role = "api"
	RoleWorker     Role = "worker"
	RoleScheduler  Role = "scheduler"
	RoleAutomation Role = "automation"
)

// TenancyMode distinguishes self-hosting from provider operation (ADR-0010).
type TenancyMode string

const (
	TenancySingle TenancyMode = "single"
	TenancyMulti  TenancyMode = "multi"
)

// Config is the complete configuration state of the process.
type Config struct {
	Version   string
	Commit    string
	Roles     []Role
	HTTPAddr  string
	OpsAddr   string
	BaseURL   string
	LogFormat string
	LogLevel  string

	Tenancy TenancyMode

	DatabaseDSN secret.Secret
	SecretKey   secret.Secret

	// ShutdownGraceSeconds is the deadline for in-flight requests after SIGTERM.
	ShutdownGraceSeconds int
}

func (c Config) HasRole(r Role) bool {
	for _, have := range c.Roles {
		if have == r {
			return true
		}
	}
	return false
}

// Port loads and validates the configuration.
type Port interface {
	Load() (Config, error)
	// Warnings reports states that are not errors but that the operator is missing -
	// a missing backup configuration, for example. They appear in /meta/health (ADR-0016).
	Warnings(Config) []Warning
}

// Warning is machine readable: a code instead of free text, so that clients can translate.
type Warning struct {
	Code     string
	Severity string // info | warn | critical
	Params   map[string]string
}
