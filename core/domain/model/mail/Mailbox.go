// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Protocol is how a connected mailbox is read (ADR-0094 decision 1). There is no SMTP receiver:
// Hubtask fetches from a mailbox that already exists.
type Protocol string

const (
	// ProtocolIMAP reads over IMAP with implicit TLS, the one every provider speaks.
	ProtocolIMAP Protocol = "IMAP"
	// ProtocolJMAP reads over JMAP (RFC 8620/8621), JSON over HTTPS, where the provider speaks it.
	ProtocolJMAP Protocol = "JMAP"
)

// MaxMailboxes is how many mailboxes one workspace connects: a household's and a team's, and a
// shared support address beside them.
const MaxMailboxes = 5

// DefaultDoneFolder is where a fetched message is moved, so the mailbox shows what Hubtask took and
// nothing is fetched twice even before the receipt is read.
const DefaultDoneFolder = "Hubtask"

// PollInterval is how often a mailbox is read. A minute is "right away" for a list and gentle on a
// provider that counts logins.
const PollInterval = time.Minute

// Mailbox is a connected mailbox: where it is and how it is read. The secret - a password, an app
// password or a token - is not a field: the store keeps it sealed, and nothing reads it back but
// the job that signs in.
type Mailbox struct {
	ID       shared.ID
	TenantID shared.ID
	Protocol Protocol
	// Address is the mailbox's own address, whose plus tags route ("familie+einkauf@gmail.com").
	Address string
	// Host and Port are the IMAP server; SessionURL the JMAP session resource.
	Host       string
	Port       int
	SessionURL string
	Username   string
	// Folder is read; DoneFolder is where read messages go.
	Folder     string
	DoneFolder string
	Enabled    bool
	// Preset is the provider template it was set up from ("gmail"), empty for "other".
	Preset string
	// CheckedAt is the last read, Failure what the last read failed with (a message code, never the
	// server's own words), FailingSince when it started to fail.
	CheckedAt    *time.Time
	Failure      string
	FailingSince *time.Time
	CreatedAt    time.Time
}

// Preset is what a provider needs, so a person types an address and an app password and nothing
// else. Hint is a message code saying what the provider asks for first.
type Preset struct {
	Key        string
	Protocol   Protocol
	Host       string
	Port       int
	SessionURL string
	// PlusAddresses: the provider delivers name+tag@ to name@, so a plus tag can route.
	PlusAddresses bool
	Hint          string
}

// Presets are the providers people use, with what each asks for (verified 2026-10):
// Gmail allows IMAP only with an app password (two-step verification on) or OAuth; GMX and web.de
// need "IMAP access" switched on in their settings once; IONOS takes the mailbox password;
// Microsoft accepts OAuth only since September 2024, which is a later step; Fastmail speaks JMAP
// with an API token.
var Presets = []Preset{
	{Key: "gmail", Protocol: ProtocolIMAP, Host: "imap.gmail.com", Port: 993, PlusAddresses: true, Hint: "mail.preset_gmail"},
	{Key: "gmx", Protocol: ProtocolIMAP, Host: "imap.gmx.net", Port: 993, Hint: "mail.preset_gmx"},
	{Key: "webde", Protocol: ProtocolIMAP, Host: "imap.web.de", Port: 993, Hint: "mail.preset_webde"},
	{Key: "ionos", Protocol: ProtocolIMAP, Host: "imap.ionos.de", Port: 993, PlusAddresses: true, Hint: "mail.preset_ionos"},
	{Key: "fastmail", Protocol: ProtocolJMAP, SessionURL: "https://api.fastmail.com/jmap/session", PlusAddresses: true, Hint: "mail.preset_fastmail"},
}

// PresetNamed is a provider by key.
func PresetNamed(key string) (Preset, bool) {
	for _, preset := range Presets {
		if preset.Key == key {
			return preset, true
		}
	}
	return Preset{}, false
}

// NewMailboxInput is what a person enters.
type NewMailboxInput struct {
	ID         shared.ID
	TenantID   shared.ID
	Preset     string
	Protocol   Protocol
	Address    string
	Host       string
	Port       int
	SessionURL string
	Username   string
	Folder     string
	DoneFolder string
	Now        time.Time
}

// NewMailbox checks a mailbox before anything signs in to it: a preset fills what the person did
// not, and every field is named where it is wrong.
func NewMailbox(in NewMailboxInput) (Mailbox, error) {
	box := Mailbox{
		ID: in.ID, TenantID: in.TenantID, Protocol: in.Protocol, Preset: in.Preset,
		Address:  strings.ToLower(strings.TrimSpace(in.Address)),
		Host:     strings.ToLower(strings.TrimSpace(in.Host)),
		Port:     in.Port,
		Username: strings.TrimSpace(in.Username),
		Folder:   strings.TrimSpace(in.Folder), DoneFolder: strings.TrimSpace(in.DoneFolder),
		SessionURL: strings.TrimSpace(in.SessionURL),
		Enabled:    true, CreatedAt: in.Now.UTC(),
	}
	if preset, ok := PresetNamed(in.Preset); ok {
		box.Protocol = preset.Protocol
		if box.Host == "" {
			box.Host = preset.Host
		}
		if box.Port == 0 {
			box.Port = preset.Port
		}
		if box.SessionURL == "" {
			box.SessionURL = preset.SessionURL
		}
	}
	if box.Username == "" {
		box.Username = box.Address
	}
	if box.Folder == "" {
		box.Folder = "INBOX"
	}
	if box.DoneFolder == "" {
		box.DoneFolder = DefaultDoneFolder
	}
	if box.Port == 0 && box.Protocol == ProtocolIMAP {
		box.Port = 993
	}

	var fields []shared.FieldError
	if parsed, err := mail.ParseAddress(box.Address); err != nil || parsed.Address != box.Address {
		fields = append(fields, shared.FieldError{Path: "/address", Code: "mail.mailbox_address_invalid"})
	}
	switch box.Protocol {
	case ProtocolIMAP:
		if !validHost(box.Host) {
			fields = append(fields, shared.FieldError{Path: "/host", Code: "mail.mailbox_host_invalid"})
		}
		if box.Port < 1 || box.Port > 65535 {
			fields = append(fields, shared.FieldError{Path: "/port", Code: "mail.mailbox_port_invalid"})
		}
	case ProtocolJMAP:
		if parsed, err := url.Parse(box.SessionURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			fields = append(fields, shared.FieldError{Path: "/session_url", Code: "mail.mailbox_session_url_invalid"})
		}
	default:
		fields = append(fields, shared.FieldError{Path: "/protocol", Code: "mail.mailbox_protocol_unknown"})
	}
	if len(box.Username) > 320 || strings.ContainsAny(box.Username, "\r\n\"") {
		fields = append(fields, shared.FieldError{Path: "/username", Code: "mail.mailbox_username_invalid"})
	}
	for path, folder := range map[string]string{"/folder": box.Folder, "/done_folder": box.DoneFolder} {
		if len(folder) > 100 || strings.ContainsAny(folder, "\r\n\"*%") {
			fields = append(fields, shared.FieldError{Path: path, Code: "mail.mailbox_folder_invalid"})
		}
	}
	if len(fields) > 0 {
		return Mailbox{}, shared.ErrValidation.WithDetail("mail.mailbox_invalid").WithFields(fields...)
	}
	return box, nil
}

// Endpoint is the IMAP server as host:port.
func (m Mailbox) Endpoint() string { return m.Host + ":" + strconv.Itoa(m.Port) }

// Checked is the mailbox after a read: the time, and a failure that starts or ends.
func (m Mailbox) Checked(at time.Time, failure string) Mailbox {
	at = at.UTC()
	m.CheckedAt = &at
	switch {
	case failure == "":
		m.Failure, m.FailingSince = "", nil
	case m.FailingSince == nil:
		m.Failure, m.FailingSince = failure, &at
	default:
		m.Failure = failure
	}
	return m
}

// Failing reports whether the mailbox has failed for longer than a grace: a server that refuses one
// read is a hiccup, one that refuses for an hour is something an administrator must hear about.
func (m Mailbox) Failing(now time.Time) bool {
	return m.FailingSince != nil && now.Sub(*m.FailingSince) >= time.Hour
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /:@") {
		return false
	}
	return strings.Contains(host, ".")
}
