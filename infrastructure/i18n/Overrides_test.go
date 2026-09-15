// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func configCode(t *testing.T, err error) (string, map[string]string) {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "config_invalid" {
		t.Fatalf("not a configuration error: %v", err)
	}
	return domainErr.DetailCode, domainErr.Params
}

// A mistyped path is a translation that silently never arrives, so it refuses to start instead.
func TestAMissingDirectoryIsAConfigurationError(t *testing.T) {
	_, err := i18n.NewRendererFromConfig(env.LocaleConfig{Directory: filepath.Join(t.TempDir(), "absent")})
	code, params := configCode(t, err)
	if code != "config.locale_dir_missing" || params["variable"] != "HUBTASK_LOCALE_DIR" {
		t.Errorf("code %s, params %v", code, params)
	}
}

// The file is named, its content is not: what could not be parsed is not the log's to reprint.
func TestAFileThatIsNotACatalogueIsAConfigurationErrorNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "de.json", `{"email.reminder.subject": ["not", "a", "sentence"]}`)

	_, err := i18n.NewRendererFromConfig(env.LocaleConfig{Directory: dir})
	code, params := configCode(t, err)
	if code != "config.locale_file_invalid" || params["file"] != "de.json" {
		t.Errorf("code %s, params %v", code, params)
	}
}

// The whole of "enabling the locale" (arc42 QS-08): a file in a directory, and a restart.
func TestAnOperatorAddsALanguageWithAFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ar.json", `{"email.reminder.subject.withheld": "تذكير من Hubtask"}`)
	write(t, dir, "en.json", `{"email.reminder.subject.withheld": "A reminder, corrected by the operator"}`)

	renderer, err := i18n.NewRendererFromConfig(env.LocaleConfig{Directory: dir})
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	if got := renderer.Render("ar-EG", "email.reminder.subject.withheld", nil); got != "تذكير من Hubtask" {
		t.Errorf("the new locale did not arrive: %q", got)
	}
	if got := renderer.Render("en", "email.reminder.subject.withheld", nil); got != "A reminder, corrected by the operator" {
		t.Errorf("the override of an embedded key did not win: %q", got)
	}
	if got := renderer.Render("en", "email.reminder.subject", map[string]string{"title": "x"}); got != "Reminder: “x”" {
		t.Errorf("a key the override does not name lost its embedded sentence: %q", got)
	}
	locales := renderer.Locales()
	if locales[0] != "en" || len(locales) < 2 {
		t.Errorf("Locales() = %v", locales)
	}
}

// No directory configured is the default, and it is the embedded catalogues alone.
func TestNoDirectoryIsTheEmbeddedCatalogues(t *testing.T) {
	renderer, err := i18n.NewRendererFromConfig(env.LocaleConfig{})
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	if got := renderer.Render("en", "email.invitation.subject", nil); got != "You have been invited to Hubtask" {
		t.Errorf("rendered %q", got)
	}
}
