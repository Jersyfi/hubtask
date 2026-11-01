// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import "sort"

// PresetCount is one provider preset's mailboxes on the installation's own app, across every
// workspace: how many are enabled, and how many have failed for longer than Failing's grace.
type PresetCount struct {
	Provider           string
	Connected, Failing int64
}

// AppHealth is how one of the installation's mail apps fares (alert A-29). When its secret runs
// out, every mailbox on it fails at once, and only the operator can fix that.
type AppHealth struct {
	App                string
	Connected, Failing int64
}

// InstallationApps folds the presets' counts onto the app each signs in through, sorted by app.
// Every preset's app is named even without a mailbox, so a gauge falls to zero instead of keeping
// the last count of an app whose mailboxes all went.
func InstallationApps(counts []PresetCount) []AppHealth {
	byApp := map[string]AppHealth{}
	for _, preset := range OAuthPresets {
		app := InstallationApp(preset.Key)
		byApp[app] = AppHealth{App: app}
	}
	for _, count := range counts {
		app := InstallationApp(count.Provider)
		health := byApp[app]
		health.App = app
		health.Connected += count.Connected
		health.Failing += count.Failing
		byApp[app] = health
	}
	apps := make([]AppHealth, 0, len(byApp))
	for _, health := range byApp {
		apps = append(apps, health)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].App < apps[j].App })
	return apps
}
