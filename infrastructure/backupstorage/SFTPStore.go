// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"path"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// SFTPStore writes archives over SSH (backup-restore.md §2).
//
// Three decisions carry this adapter, and the first is the one that would otherwise be quietly
// wrong.
//
// **The host key is configuration, not something learned on the way.** There is no
// trust-on-first-use here and no way to switch the check off: a target is created through an API,
// possibly by somebody who is not the operator, and a first connection that accepts whatever key
// answers is a first connection an attacker only has to be present for once. So a target names
// either the host's public key or its SHA-256 fingerprint, and a target that names neither is
// refused at creation. That is stricter than most tools and it is the right strictness for a
// server that reads its configuration out of a database.
//
// **The dial goes through the same guard the HTTP adapters use.** SSH is not HTTP, so rule 6's
// wording does not reach it, but the reason behind rule 6 does: this is an outbound address that
// arrived through the API, and 169.254.169.254 answers on port 22 as readily as on port 80 if
// somebody puts a listener there. The guard's resolver and its dial-time control are used
// directly, which is what makes BK-9 one test rather than two.
//
// **A connection lives for one operation.** Dialling per call costs a handshake; keeping a pool
// would cost a reconnection strategy, a liveness check and a shutdown path, for a target that is
// used a handful of times a day on a schedule. The exception is Get, whose stream outlives the
// call - so the reader it answers owns the connection and closes it.
type SFTPStore struct {
	guard   *httpclient.Guard
	address string
	// root is the target's directory on the server, absolute and without a trailing slash.
	root      string
	config    *ssh.ClientConfig
	dialTimes time.Duration
}

var _ port.Store = (*SFTPStore)(nil)

// defaultSFTPPort is what a target that names no port means.
const defaultSFTPPort = "22"

// NewSFTPStore builds the adapter from a target's configuration and credentials.
func NewSFTPStore(
	guard *httpclient.Guard, spec port.Spec, timeout time.Duration,
) (*SFTPStore, error) {
	host := spec.Config.Get("host")
	if host == "" {
		return nil, configInvalid("host", "backup.config_required")
	}
	sshPort := spec.Config.Get("port")
	if sshPort == "" {
		sshPort = defaultSFTPPort
	}

	root := strings.TrimRight(spec.Config.Get("path"), "/")
	if root == "" {
		return nil, configInvalid("path", "backup.config_required")
	}

	verify, err := hostKeyVerifier(spec)
	if err != nil {
		return nil, err
	}
	methods, err := authMethods(spec)
	if err != nil {
		return nil, err
	}

	return &SFTPStore{
		guard:   guard,
		address: net.JoinHostPort(host, sshPort),
		root:    root,
		config: &ssh.ClientConfig{
			User:            spec.Config.Get("username"),
			Auth:            methods,
			HostKeyCallback: verify,
			Timeout:         timeout,
		},
		dialTimes: timeout,
	}, nil
}

// hostKeyVerifier builds the check from what the target names, and refuses a target that names
// nothing.
//
// Two spellings, because operators have one or the other to hand: the whole public key, as it
// appears in `known_hosts` or `ssh-keyscan` output, or its SHA-256 fingerprint, as `ssh-keygen
// -lf` prints it. Both are compared against what the server presents; neither is stored back, so
// a key that changes is a refused connection rather than a silent re-trust.
func hostKeyVerifier(spec port.Spec) (ssh.HostKeyCallback, error) {
	if authorized := spec.Config.Get("host_key"); authorized != "" {
		expected, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorized))
		if err != nil {
			return nil, configInvalid("host_key", "backup.host_key_invalid")
		}
		wanted := expected.Marshal()
		return func(_ string, _ net.Addr, presented ssh.PublicKey) error {
			if !equalBytes(wanted, presented.Marshal()) {
				return errors.New("the target presented a host key that is not the configured one")
			}
			return nil
		}, nil
	}

	if fingerprint := spec.Config.Get("host_key_fingerprint"); fingerprint != "" {
		wanted := strings.TrimSpace(fingerprint)
		if !strings.HasPrefix(wanted, "SHA256:") {
			return nil, configInvalid("host_key_fingerprint", "backup.host_key_invalid")
		}
		return func(_ string, _ net.Addr, presented ssh.PublicKey) error {
			sum := sha256.Sum256(presented.Marshal())
			got := "SHA256:" + strings.TrimRight(
				base64.StdEncoding.EncodeToString(sum[:]), "=")
			if got != strings.TrimRight(wanted, "=") {
				return errors.New("the target presented a host key that is not the configured one")
			}
			return nil
		}, nil
	}

	// No trust on first use. A target is created through an API, and a first connection that
	// accepted whatever answered is a first connection somebody only has to be present for once.
	return nil, shared.ErrValidation.
		WithDetail("backup.target_invalid").
		WithFields(shared.FieldError{
			Path: "/config/host_key", Code: "backup.host_key_required",
		})
}

// authMethods is how this client proves who it is: a key, a password, or both offered in that
// order.
func authMethods(spec port.Spec) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if key := spec.Credential("private_key"); !key.IsEmpty() {
		var signer ssh.Signer
		var err error
		if passphrase := spec.Credential("private_key_passphrase"); !passphrase.IsEmpty() {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(
				[]byte(key.Reveal()), []byte(passphrase.Reveal()))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(key.Reveal()))
		}
		if err != nil {
			// The error is not wrapped: a key parser's message can quote the key's header and,
			// on some paths, its body (rule 10).
			return nil, shared.ErrValidation.
				WithDetail("backup.target_invalid").
				WithFields(shared.FieldError{
					Path: "/credentials/private_key", Code: "backup.private_key_invalid",
				})
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if password := spec.Credential("password"); !password.IsEmpty() {
		plaintext := password.Reveal()
		methods = append(methods, ssh.Password(plaintext))
	}

	if len(methods) == 0 {
		return nil, shared.ErrValidation.
			WithDetail("backup.target_invalid").
			WithFields(shared.FieldError{
				Path: "/credentials", Code: "backup.credentials_required",
			})
	}
	return methods, nil
}

// Put writes the archive, creating the directories above it first.
func (s *SFTPStore) Put(ctx context.Context, key string, content io.Reader) (int64, error) {
	if err := CheckKey(key); err != nil {
		return 0, err
	}

	session, closer, err := s.connect(ctx)
	if err != nil {
		return 0, err
	}
	defer closer()

	if err := s.ensureDirectories(session, key); err != nil {
		return 0, err
	}

	handle, err := session.open(s.pathOf(key), openWrite|openCreate|openTruncate)
	if err != nil {
		return 0, s.translate("writing the archive", key, err)
	}
	defer func() { _ = session.closeHandle(handle) }()

	var offset uint64
	buffer := make([]byte, chunkSize)
	for {
		read, err := content.Read(buffer)
		if read > 0 {
			if err := session.writeAt(handle, offset, buffer[:read]); err != nil {
				return 0, s.translate("writing the archive", key, err)
			}
			offset += uint64(read)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, failed("reading the archive", err)
		}
	}
	return int64(offset), nil
}

// Get opens the archive. The reader owns the connection and closes it, because the stream
// outlives this call.
func (s *SFTPStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := CheckKey(key); err != nil {
		return nil, err
	}

	session, closer, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}

	handle, err := session.open(s.pathOf(key), openRead)
	if err != nil {
		closer()
		return nil, s.translate("reading the archive", key, err)
	}
	return &sftpReader{session: session, handle: handle, closer: closer}, nil
}

// List walks the prefix. SFTP has directories rather than a flat namespace, so the walk is this
// adapter's and the keys come back whole.
func (s *SFTPStore) List(ctx context.Context, prefix string) ([]port.Entry, error) {
	if err := CheckPrefix(prefix); err != nil {
		return nil, err
	}

	session, closer, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer closer()

	return s.walk(session, strings.TrimSuffix(prefix, "/"), 0)
}

func (s *SFTPStore) walk(
	session *sftpSession, prefix string, depth int,
) ([]port.Entry, error) {
	if depth > maxCollectionDepth {
		return nil, nil
	}

	names, err := session.readdir(s.pathOf(prefix))
	if err != nil {
		var status sftpError
		if errors.As(err, &status) && status.missing() {
			// A directory nothing has been written into is an empty listing: the answer a fresh
			// target gives.
			return nil, nil
		}
		return nil, s.translate("listing the target", prefix, err)
	}

	var entries []port.Entry
	for _, name := range names {
		if name.name == "." || name.name == ".." {
			continue
		}
		key := strings.TrimPrefix(path.Join(prefix, name.name), "/")

		if name.isDir {
			below, err := s.walk(session, key, depth+1)
			if err != nil {
				return nil, err
			}
			entries = append(entries, below...)
			continue
		}
		entries = append(entries, port.Entry{
			Key: key, Size: name.size, ModifiedAt: name.modified,
		})
	}
	return entries, nil
}

// Stat answers the archive's size and age without reading it.
func (s *SFTPStore) Stat(ctx context.Context, key string) (port.Entry, error) {
	if err := CheckKey(key); err != nil {
		return port.Entry{}, err
	}

	session, closer, err := s.connect(ctx)
	if err != nil {
		return port.Entry{}, err
	}
	defer closer()

	info, err := session.stat(s.pathOf(key))
	if err != nil {
		return port.Entry{}, s.translate("measuring the archive", key, err)
	}
	if info.isDir {
		return port.Entry{}, notFound(key)
	}
	return port.Entry{Key: key, Size: info.size, ModifiedAt: info.modified}, nil
}

// Delete removes the archive. Removing what is not there succeeds.
func (s *SFTPStore) Delete(ctx context.Context, key string) error {
	if err := CheckKey(key); err != nil {
		return err
	}

	session, closer, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer closer()

	if err := session.remove(s.pathOf(key)); err != nil {
		var status sftpError
		if errors.As(err, &status) && status.missing() {
			return nil
		}
		return s.translate("removing the archive", key, err)
	}
	return nil
}

// ensureDirectories makes the directories above a key. A directory that is already there answers
// a failure rather than a distinct code in version 3, so the answer is checked instead of the
// code: if it is a directory afterwards, what was asked for has happened.
func (s *SFTPStore) ensureDirectories(session *sftpSession, key string) error {
	segments := strings.Split(key, "/")
	for index := range len(segments) - 1 {
		directory := s.pathOf(strings.Join(segments[:index+1], "/"))

		if err := session.mkdir(directory); err != nil {
			info, statErr := session.stat(directory)
			if statErr != nil || !info.isDir {
				return s.translate("preparing the directory", key, err)
			}
		}
	}
	return nil
}

// connect dials, checks the address against the guard, and opens the SFTP subsystem.
func (s *SFTPStore) connect(ctx context.Context) (*sftpSession, func(), error) {
	host, _, err := net.SplitHostPort(s.address)
	if err != nil {
		return nil, nil, configInvalid("host", "backup.config_required")
	}
	// The same check the HTTP adapters get from GuardedClient, applied by hand because SSH is not
	// HTTP: resolution first, so a target inside the network fails before a socket is opened.
	if _, err := s.guard.Resolve(ctx, host); err != nil {
		return nil, nil, err
	}

	dialer := &net.Dialer{
		Timeout: s.dialTimes,
		// The dial-time control, which is the check that cannot be raced: the name may resolve
		// to something allowed and the connection still be made to something else.
		Control: func(network, address string, _ syscall.RawConn) error {
			return s.guard.CheckControl(network, address)
		},
	}
	connection, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return nil, nil, unreachable("reaching the target", err)
	}

	// The handshake gets the same deadline as the dial: an SSH server that accepts a connection
	// and then says nothing would otherwise hold this call for as long as it liked (rule 7).
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(s.dialTimes))
	}

	client, chans, reqs, err := ssh.NewClientConn(connection, s.address, s.config)
	if err != nil {
		_ = connection.Close()
		// A host key that does not match and a password that does not work are both refusals.
		// The library's message quotes neither, but it does quote the address, so it is coded
		// rather than carried.
		return nil, nil, refused("authenticating at the target", errors.New("the handshake failed"))
	}
	// Cleared: past the handshake the deadline would cut a long transfer in half, and the
	// caller's context is what bounds the operation from here on.
	_ = connection.SetDeadline(time.Time{})

	connected := ssh.NewClient(client, chans, reqs)
	session, err := connected.NewSession()
	if err != nil {
		_ = connected.Close()
		return nil, nil, failed("opening the session", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = connected.Close()
		return nil, nil, failed("opening the session", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = connected.Close()
		return nil, nil, failed("opening the session", err)
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		_ = connected.Close()
		return nil, nil, refused("asking for the sftp subsystem", err)
	}

	sftp, err := newSFTPSession(&pipe{reader: stdout, writer: stdin})
	if err != nil {
		_ = connected.Close()
		return nil, nil, failed("starting the sftp conversation", err)
	}
	return sftp, func() { _ = connected.Close() }, nil
}

// pathOf is where a key lives on the server.
func (s *SFTPStore) pathOf(key string) string {
	if key == "" {
		return s.root
	}
	return s.root + "/" + key
}

// translate turns the server's status into the port's vocabulary.
func (s *SFTPStore) translate(doing, key string, err error) error {
	var status sftpError
	if errors.As(err, &status) {
		switch {
		case status.missing():
			return notFound(key)
		case status.refused():
			return refused(doing, err)
		}
	}
	return failed(doing, err)
}

// sftpReader streams one archive and owns the connection it came over.
type sftpReader struct {
	session *sftpSession
	handle  []byte
	closer  func()
	offset  uint64
	buffer  []byte
	done    bool
}

func (r *sftpReader) Read(p []byte) (int, error) {
	for len(r.buffer) == 0 {
		if r.done {
			return 0, io.EOF
		}
		chunk, err := r.session.readAt(r.handle, r.offset, chunkSize)
		if errors.Is(err, io.EOF) {
			r.done = true
			return 0, io.EOF
		}
		if err != nil {
			return 0, failed("reading the archive", err)
		}
		r.offset += uint64(len(chunk))
		r.buffer = chunk
	}

	copied := copy(p, r.buffer)
	r.buffer = r.buffer[copied:]
	return copied, nil
}

func (r *sftpReader) Close() error {
	err := r.session.closeHandle(r.handle)
	r.closer()
	return err
}

// pipe joins the session's two halves into the one stream the protocol client wants.
type pipe struct {
	reader io.Reader
	writer io.WriteCloser
}

func (p *pipe) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p *pipe) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p *pipe) Close() error                { return p.writer.Close() }

// equalBytes compares two public keys. Not constant time, and deliberately: both values are
// public, and the comparison decides whether to talk to the machine that presented one of them.
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
