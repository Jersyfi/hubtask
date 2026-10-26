// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"crypto/sha256"
	"encoding/base64"
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
	// Auth is how it signs in; OAuth names the app when it is OAuth.
	Auth  Auth
	OAuth *OAuthClient
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
		Enabled:    true, CreatedAt: in.Now.UTC(), Auth: AuthPassword,
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

// Auth is how a mailbox signs in.
type Auth string

const (
	// AuthPassword signs in with a password or an app password: any IMAP server.
	AuthPassword Auth = "PASSWORD"
	// AuthOAuth signs in with an OAuth access token (IMAP SASL XOAUTH2), drawn from a refresh token:
	// Google and Microsoft by preset, any other provider by its endpoints.
	AuthOAuth Auth = "OAUTH"
)

// OAuthClient is the OAuth app a mailbox signs in through: where to authorize, where to trade a code
// and refresh a token, what to ask for, and whose app it is. The client secret is not a field - it
// is sealed beside the mailbox, or the installation's.
type OAuthClient struct {
	// Provider is "google", "microsoft" or "custom".
	Provider     string
	AuthorizeURL string
	TokenURL     string
	Scopes       []string
	ClientID     string
	// Installation is whether the operator's app is used (its secret in the configuration), rather
	// than one the administrator registered and entered.
	Installation bool
}

// OAuthPreset is a provider's endpoints and its IMAP server, so connecting is one button.
type OAuthPreset struct {
	Key          string
	AuthorizeURL string
	TokenURL     string
	Scopes       []string
	Host         string
	Port         int
	// Params are what the provider needs to hand out a refresh token at all.
	Params map[string]string
	Hint   string
}

// OAuthPresets are the providers whose mailboxes sign in with OAuth (verified 2026-10): Google -
// IMAP with the full mail scope, a refresh token only with offline access and consent; Microsoft -
// work, school and personal accounts through the common endpoint, IMAP's own scope.
var OAuthPresets = []OAuthPreset{
	{ //nolint:gosec // G101: a provider's public endpoints, no credential
		Key: "google", AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		Scopes:   []string{"https://mail.google.com/", "openid", "email"},
		Host:     "imap.gmail.com", Port: 993,
		Params: map[string]string{"access_type": "offline", "prompt": "consent"},
		Hint:   "mail.oauth_google",
	},
	{ //nolint:gosec // G101: a provider's public endpoints, no credential
		Key: "microsoft", AuthorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		Scopes:   []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "offline_access", "openid", "email"},
		Host:     "outlook.office365.com", Port: 993,
		Params: map[string]string{"prompt": "select_account"},
		Hint:   "mail.oauth_microsoft",
	},
}

// OAuthPresetNamed is a provider by key.
func OAuthPresetNamed(key string) (OAuthPreset, bool) {
	for _, preset := range OAuthPresets {
		if preset.Key == key {
			return preset, true
		}
	}
	return OAuthPreset{}, false
}

// OAuthRequest is what an administrator asks for: a preset, or a provider of their own.
type OAuthRequest struct {
	Provider     string
	AuthorizeURL string
	TokenURL     string
	Scopes       []string
	ClientID     string
	Host         string
	Port         int
	// Installation says the operator's app is configured for the preset and is to be used.
	Installation bool
}

// NewOAuthClient checks an OAuth request: a preset fills its endpoints, a provider of one's own
// must name both over HTTPS, and a client is named either way.
func NewOAuthClient(in OAuthRequest) (OAuthClient, string, int, error) {
	client := OAuthClient{
		Provider: strings.TrimSpace(in.Provider), AuthorizeURL: strings.TrimSpace(in.AuthorizeURL),
		TokenURL: strings.TrimSpace(in.TokenURL), ClientID: strings.TrimSpace(in.ClientID),
		Installation: in.Installation,
	}
	for _, scope := range in.Scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			client.Scopes = append(client.Scopes, scope)
		}
	}
	host, port := strings.ToLower(strings.TrimSpace(in.Host)), in.Port
	if preset, ok := OAuthPresetNamed(client.Provider); ok {
		client.AuthorizeURL, client.TokenURL, client.Scopes = preset.AuthorizeURL, preset.TokenURL, preset.Scopes
		host, port = preset.Host, preset.Port
	} else {
		client.Provider = "custom"
	}
	if port == 0 {
		port = 993
	}
	var fields []shared.FieldError
	for path, address := range map[string]string{"/authorize_url": client.AuthorizeURL, "/token_url": client.TokenURL} {
		if parsed, err := url.Parse(address); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			fields = append(fields, shared.FieldError{Path: path, Code: "mail.oauth_url_invalid"})
		}
	}
	if len(client.Scopes) == 0 {
		fields = append(fields, shared.FieldError{Path: "/scopes", Code: "mail.oauth_scopes_required"})
	}
	if !client.Installation && (client.ClientID == "" || len(client.ClientID) > 500) {
		fields = append(fields, shared.FieldError{Path: "/client_id", Code: "mail.oauth_client_required"})
	}
	if !validHost(host) {
		fields = append(fields, shared.FieldError{Path: "/host", Code: "mail.mailbox_host_invalid"})
	}
	if port < 1 || port > 65535 {
		fields = append(fields, shared.FieldError{Path: "/port", Code: "mail.mailbox_port_invalid"})
	}
	if len(fields) > 0 {
		return OAuthClient{}, "", 0, shared.ErrValidation.WithDetail("mail.oauth_invalid").WithFields(fields...)
	}
	return client, host, port, nil
}

// PendingTTL is how long an administrator has between leaving for the provider and coming back.
const PendingTTL = 10 * time.Minute

// PendingAuthorization is a connection waiting for the provider's answer: who asked, for what, and
// until when. It is used once.
type PendingAuthorization struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	Client    OAuthClient
	Host      string
	Port      int
	// LoginHint is the address the administrator typed, sent to the provider and kept as the
	// mailbox's address where the provider names none.
	LoginHint string
	ExpiresAt time.Time
}

// Expired reports whether the provider took too long to answer.
func (p PendingAuthorization) Expired(now time.Time) bool { return !now.Before(p.ExpiresAt) }

// AuthorizationURL is where the administrator's browser goes to say yes: the authorization code
// flow with PKCE S256 (RFC 7636), the state that brings the answer back to this connection, and the
// address typed as a hint so the provider asks for the right account.
func (c OAuthClient) AuthorizationURL(redirectURL, state, verifier, loginHint string) string {
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.ClientID},
		"redirect_uri":          {redirectURL},
		"scope":                 {strings.Join(c.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	if loginHint != "" {
		query.Set("login_hint", loginHint)
	}
	if preset, ok := OAuthPresetNamed(c.Provider); ok {
		for key, value := range preset.Params {
			query.Set(key, value)
		}
	}
	separator := "?"
	if strings.Contains(c.AuthorizeURL, "?") {
		separator = "&"
	}
	return c.AuthorizeURL + separator + query.Encode()
}
