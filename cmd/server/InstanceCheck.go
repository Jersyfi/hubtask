// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"io"
	"os"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	servicedeskservice "github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/instancefile"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// runInstanceCommand is `hubtask instance check [file]`: every reader the start runs over the
// instance file, without a database, so a typo is found before the restart it would refuse
// (deployment.md §6.1). The file defaults to HUBTASK_INSTANCE_FILE; the environment variables and
// secret files the file names are read as the server reads them, so it runs where the server runs.
func runInstanceCommand(args []string, stdout, stderr io.Writer) int {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "message catalogue: %v\n", err)
		return 1
	}
	locale := os.Getenv("HUBTASK_DEFAULT_LOCALE")
	if locale == "" {
		locale = "en"
	}
	render := func(code string, params map[string]string) string { return renderer.Render(locale, code, params) }
	return instanceCommand(args, stdout, stderr, render, os.LookupEnv, os.ReadFile)
}

func instanceCommand(
	args []string, stdout, stderr io.Writer, render func(string, map[string]string) string,
	lookup func(string) (string, bool), readFile func(string) ([]byte, error),
) int {
	if len(args) == 0 || args[0] != "check" || len(args) > 2 {
		_, _ = fmt.Fprintln(stderr, render("cli.instance_usage", nil))
		return 2
	}
	path, _ := lookup("HUBTASK_INSTANCE_FILE")
	if len(args) == 2 {
		path = args[1]
	}
	if path == "" {
		_, _ = fmt.Fprintln(stderr, render("cli.instance_usage", nil))
		return 2
	}
	refusals := checkInstanceFile(path, lookup, readFile)
	if len(refusals) == 0 {
		_, _ = fmt.Fprintln(stdout, render("cli.instance_file_valid", map[string]string{"source": path}))
		return 0
	}
	for _, refusal := range refusals {
		failure := shared.AsError(refusal)
		code := failure.DetailCode
		if code == "" {
			code = "config.instance_file_malformed"
		}
		params := failure.Params
		if params == nil {
			params = map[string]string{"source": path}
		}
		_, _ = fmt.Fprintln(stderr, render(code, params))
		for _, field := range failure.Fields {
			_, _ = fmt.Fprintf(stderr, "  %s: %s\n", field.Path, render(field.Code, field.Params))
		}
	}
	return 1
}

// checkInstanceFile runs the start's readers and answers each refusal; a file that does not exist
// is one, because a check names the file it is asked about. What only the database can judge - a
// plan against the installation's values, a preset's settings - is the start's to refuse.
func checkInstanceFile(
	path string, lookup func(string) (string, bool), readFile func(string) ([]byte, error),
) []error {
	document, found, err := instancefile.Read(path)
	if err != nil {
		return []error{err}
	}
	if !found {
		return []error{shared.ErrValidation.WithDetail("config.instance_file_unreadable").
			WithParams(map[string]string{"source": path})}
	}
	var refusals []error
	note := func(err error) {
		if err != nil {
			refusals = append(refusals, err)
		}
	}
	_, _, err = instancefile.AiModels(document, lookup, path)
	note(err)
	_, err = adminservice.InstanceLevelOf(document)
	note(err)
	_, _, err = instancefile.Plans(document, path)
	note(err)
	_, _, err = instancefile.Presets(document, path)
	note(err)
	_, _, err = instancefile.PlatformSubscriptions(document, readFile, path)
	note(err)
	_, err = instancefile.UploadScanner(document, path)
	note(err)
	if entries, err := instancefile.BusinessCalendars(document, path); err != nil {
		note(err)
	} else {
		_, err = servicedeskservice.InstallationCalendars(entries, textadapter.Forms{})
		note(err)
	}
	return refusals
}
