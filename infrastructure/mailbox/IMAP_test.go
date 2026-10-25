// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var connected = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

type stored struct {
	arrived time.Time
	raw     string
	folder  string
	deleted bool
}

// server is an IMAP server in the test process: the commands a read sends, as Gmail and GMX
// answer them.
type server struct {
	mu        sync.Mutex
	password  string
	noMove    bool
	messages  map[int]*stored
	folders   map[string]bool
	commands  []string
	literalOK bool
}

func newServer(password string) *server {
	return &server{password: password, folders: map[string]bool{"INBOX": true}, messages: map[int]*stored{
		1: {arrived: connected.Add(-48 * time.Hour), raw: "Subject: old\r\n\r\nalready there", folder: "INBOX"},
		2: {arrived: connected.Add(10 * time.Minute), raw: "Subject: Milch #einkauf\r\n\r\nbitte", folder: "INBOX"},
		3: {arrived: connected.Add(20 * time.Minute), raw: "Subject: [WEB-42]\r\n\r\nerledigt", folder: "INBOX"},
	}}
}

func (s *server) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(format string, args ...any) { fmt.Fprintf(conn, format+"\r\n", args...) }
	write("* OK IMAP4rev1 ready")
	selected := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		// A synchronising literal: answer the continuation, read the bytes and the rest of the line.
		for strings.HasSuffix(line, "}") {
			open := strings.LastIndex(line, "{")
			size, _ := strconv.Atoi(line[open+1 : len(line)-1])
			write("+ go ahead")
			data := make([]byte, size)
			_, _ = reader.Read(data)
			rest, _ := reader.ReadString('\n')
			line = line[:open] + `"` + string(data) + `"` + strings.TrimRight(rest, "\r\n")
			s.literalOK = true
		}
		tag, text, _ := strings.Cut(line, " ")
		s.mu.Lock()
		s.commands = append(s.commands, text)
		s.mu.Unlock()
		verb := strings.ToUpper(strings.Fields(text)[0])
		if verb == "UID" {
			verb += " " + strings.ToUpper(strings.Fields(text)[1])
		}
		switch verb {
		case "LOGIN":
			if !strings.HasSuffix(text, `"`+s.password+`"`) {
				write("%s NO [AUTHENTICATIONFAILED] Invalid credentials", tag)
				continue
			}
			write("%s OK logged in", tag)
		case "SELECT":
			name := strings.Trim(strings.TrimPrefix(text, "SELECT "), `"`)
			if !s.folders[name] {
				write("%s NO no such folder", tag)
				continue
			}
			selected = name
			write("%s OK [READ-WRITE] selected", tag)
		case "UID SEARCH":
			var uids []string
			for _, uid := range s.sorted(selected) {
				if !s.messages[uid].arrived.Before(connected.Truncate(24 * time.Hour)) {
					uids = append(uids, strconv.Itoa(uid))
				}
			}
			write("* SEARCH %s", strings.Join(uids, " "))
			write("%s OK search done", tag)
		case "UID FETCH":
			fields := strings.Fields(text)
			for _, raw := range strings.Split(fields[2], ",") {
				uid, _ := strconv.Atoi(raw)
				message, ok := s.messages[uid]
				if !ok || message.folder != selected {
					continue
				}
				if strings.Contains(text, "BODY.PEEK[]") {
					write("* %d FETCH (UID %d BODY[] {%d}", uid, uid, len(message.raw))
					_, _ = conn.Write([]byte(message.raw))
					write(")")
				} else {
					write(`* %d FETCH (UID %d INTERNALDATE "%s" RFC822.SIZE %d)`, uid, uid,
						message.arrived.Format("2-Jan-2006 15:04:05 -0700"), len(message.raw))
				}
			}
			write("%s OK fetch done", tag)
		case "CREATE":
			name := strings.Trim(strings.TrimPrefix(text, "CREATE "), `"`)
			if s.folders[name] {
				write("%s NO [ALREADYEXISTS] exists", tag)
				continue
			}
			s.folders[name] = true
			write("%s OK created", tag)
		case "UID MOVE":
			if s.noMove {
				write("%s BAD unknown command", tag)
				continue
			}
			fields := strings.Fields(text)
			uid, _ := strconv.Atoi(fields[2])
			s.messages[uid].folder = strings.Trim(fields[3], `"`)
			write("%s OK moved", tag)
		case "UID COPY":
			write("%s OK copied", tag)
		case "UID STORE":
			fields := strings.Fields(text)
			uid, _ := strconv.Atoi(fields[2])
			s.messages[uid].deleted = true
			write("%s OK stored", tag)
		case "UID EXPUNGE":
			for _, message := range s.messages {
				if message.deleted {
					message.folder = "Hubtask"
				}
			}
			write("%s OK expunged", tag)
		case "LOGOUT":
			write("* BYE")
			write("%s OK bye", tag)
			return
		default:
			write("%s BAD unknown", tag)
		}
	}
}

func (s *server) sorted(folder string) []int {
	var uids []int
	for uid, message := range s.messages {
		if message.folder == folder {
			uids = append(uids, uid)
		}
	}
	sort.Ints(uids)
	return uids
}

func (s *server) reader() IMAP {
	return IMAP{Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		client, peer := net.Pipe()
		concurrency.Go(ctx, "imap-test-server", func(context.Context) { s.serve(peer) })
		return client, nil
	}}
}

func connectionFor(password, folder string) port.Connection {
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: "m1", TenantID: "t1", Preset: "gmail", Address: "familie@gmail.com", Folder: folder, Now: connected,
	})
	if err != nil {
		panic(err)
	}
	return port.Connection{Mailbox: box, Secret: secret.New(password)}
}

func TestOnlyWhatArrivedSinceConnectingIsTakenAndMovedOnce(t *testing.T) {
	server := newServer("app-password")
	session, err := server.reader().Open(context.Background(), connectionFor("app-password", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	messages, err := session.Fetch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Ref != "2" || !strings.Contains(string(messages[0].Raw), "#einkauf") {
		t.Fatalf("fetched %+v", messages)
	}
	for _, message := range messages {
		if err := session.Done(context.Background(), message.Ref); err != nil {
			t.Fatal(err)
		}
	}
	if server.messages[2].folder != "Hubtask" || server.messages[3].folder != "Hubtask" || server.messages[1].folder != "INBOX" {
		t.Errorf("folders %v %v %v", server.messages[1].folder, server.messages[2].folder, server.messages[3].folder)
	}
	if again, _ := session.Fetch(context.Background(), 10); len(again) != 0 {
		t.Errorf("a second read took %d again", len(again))
	}
	limited, _ := newServer("app-password").reader().Open(context.Background(), connectionFor("app-password", ""))
	if few, _ := limited.Fetch(context.Background(), 1); len(few) != 1 {
		t.Errorf("the limit took %d", len(few))
	}
}

func TestAServerWithoutMoveCopiesAndExpunges(t *testing.T) {
	server := newServer("pw")
	server.noMove = true
	session, err := server.reader().Open(context.Background(), connectionFor("pw", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Done(context.Background(), "2"); err != nil {
		t.Fatal(err)
	}
	if err := session.Done(context.Background(), "3"); err != nil {
		t.Fatal(err)
	}
	if server.messages[2].folder != "Hubtask" || !server.messages[2].deleted {
		t.Errorf("message 2 %+v", server.messages[2])
	}
	moves := 0
	for _, command := range server.commands {
		if strings.HasPrefix(command, "UID MOVE") {
			moves++
		}
	}
	if moves != 1 {
		t.Errorf("MOVE was tried %d times; once is enough to learn it is missing", moves)
	}
	if err := session.Done(context.Background(), "x"); err == nil {
		t.Error("a reference that is no UID was taken")
	}
}

func TestARefusedPasswordAndAMissingFolderAreSaidApart(t *testing.T) {
	_, err := newServer("right").reader().Open(context.Background(), connectionFor("wrong", ""))
	if shared.AsError(err).DetailCode != "mail.mailbox_refused" {
		t.Errorf("a wrong password answered %v", err)
	}
	_, err = newServer("right").reader().Open(context.Background(), connectionFor("right", "Nirgends"))
	if shared.AsError(err).DetailCode != "mail.mailbox_folder_missing" {
		t.Errorf("a missing folder answered %v", err)
	}
	unreachableReader := IMAP{Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, unreachable(errors.New("connection refused"))
	}}
	_, err = unreachableReader.Open(context.Background(), connectionFor("right", ""))
	if shared.AsError(err).DetailCode != "mail.mailbox_unreachable" {
		t.Errorf("an unreachable server answered %v", err)
	}
	if _, err := (IMAP{}).Open(context.Background(), connectionFor("right", "")); err == nil {
		t.Error("an unwired reader opened")
	}
}

func TestAPasswordWithAnUmlautGoesAsALiteral(t *testing.T) {
	server := newServer("Grüße2026")
	session, err := server.reader().Open(context.Background(), connectionFor("Grüße2026", ""))
	if err != nil {
		t.Fatalf("an umlaut password: %v", err)
	}
	_ = session.Close()
	if !server.literalOK {
		t.Error("the password was not sent as a literal")
	}
}

func TestTheProtocolsSmallReaders(t *testing.T) {
	if size, ok := literalSize("* 2 FETCH (BODY[] {123}"); !ok || size != 123 {
		t.Errorf("literal %d %v", size, ok)
	}
	if _, ok := literalSize("* OK done"); ok {
		t.Error("a line without a literal had one")
	}
	uid, arrived, size, ok := describedMessage(`* 3 FETCH (UID 17 INTERNALDATE " 5-Oct-2026 10:00:00 +0200" RFC822.SIZE 2048)`)
	if !ok || uid != "17" || size != 2048 || arrived.UTC().Hour() != 8 {
		t.Errorf("described %s %v %d %v", uid, arrived, size, ok)
	}
	if quote(`a"b\c`) != `"a\"b\\c"` {
		t.Errorf("quoted %s", quote(`a"b\c`))
	}
}
