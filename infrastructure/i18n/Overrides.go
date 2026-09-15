// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"errors"
	"os"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// NewRendererFromConfig builds the renderer the installation is configured for: the embedded
// catalogues, with the operator's directory laid over them when one is named (i18n-l10n.md §1).
//
// Read once, here, at start. Nothing watches the directory: a translation changes with a
// restart, like every other piece of configuration, and a catalogue that changed under a running
// process would be one whose sentences differ from one email to the next.
func NewRendererFromConfig(locale env.LocaleConfig) (Renderer, error) {
	if locale.Directory == "" {
		return NewRenderer()
	}
	overrides, err := LoadOverrides(locale.Directory)
	if err != nil {
		return Renderer{}, err
	}
	return NewRendererWithOverrides(overrides)
}

// LoadOverrides reads an operator's catalogue directory.
//
// What it refuses, it refuses as a configuration error with a code an operator reads in the log
// and a client could translate, the way every other startup refusal is shaped: the directory that
// is not there, and a file that is not a catalogue - named by its file name and never by its
// content, because the content is what could not be parsed.
func LoadOverrides(directory string) (map[string]Catalogue, error) {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return nil, configError("config.locale_dir_missing", map[string]string{"value": directory})
	}

	catalogues, err := LoadDirectory(os.DirFS(directory))
	if err != nil {
		return nil, configError("config.locale_file_invalid", map[string]string{
			"file": offendingFile(err),
		})
	}
	return catalogues, nil
}

// configError is the shape infrastructure/environment gives a startup refusal: category
// VALIDATION, the stable code config_invalid, a detail code and the variable. Repeated here rather
// than imported, because that package's is unexported and a dependency on the environment adapter
// for one constructor would be the wrong direction between two adapters.
func configError(detailCode string, params map[string]string) *shared.Error {
	merged := map[string]string{"variable": "HUBTASK_LOCALE_DIR"}
	for name, value := range params {
		merged[name] = value
	}
	return shared.New(shared.CategoryValidation, "config_invalid").
		WithDetail(detailCode).
		WithParams(merged)
}

// offendingFile pulls the file name out of a directory reader's error, which names it as
// "reading the message catalogue <name>: …". A parameter rather than the whole error, because the
// rest of the error may quote the content that failed to parse (rule 10 is about user content,
// but an operator's file is not the log's to reprint either).
func offendingFile(err error) string {
	message := err.Error()
	const prefix = "reading the message catalogue "
	if cut := strings.Index(message, prefix); cut >= 0 {
		rest := message[cut+len(prefix):]
		if name, _, found := strings.Cut(rest, ":"); found {
			return name
		}
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Path
	}
	return ""
}
