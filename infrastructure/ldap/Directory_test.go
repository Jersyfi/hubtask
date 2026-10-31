// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package ldap

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	goldap "github.com/go-ldap/ldap/v3"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ldap"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func directoryAt(t *testing.T, url string) identity.LdapDirectory {
	t.Helper()
	d, err := identity.NewLdapDirectory("t", identity.LdapDirectoryInput{
		URL: url, BindDN: "cn=reader,dc=example,dc=com", UserBase: "ou=people,dc=example,dc=com",
		GroupBase: "ou=groups,dc=example,dc=com", IDAttribute: "objectGUID", RunAs: "acc",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func detail(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return ""
}

// A directory that refuses StartTLS is unreachable, and the bind - the password - is never sent.
func TestAPlainLdapAddressNeverBindsWithoutStartTLS(t *testing.T) {
	client, server := net.Pipe()
	received := make(chan int, 4)
	done := make(chan struct{})
	t.Cleanup(func() { _ = client.Close(); _ = server.Close(); <-done })
	concurrency.Go(context.Background(), "ldap-test-server", func(context.Context) {
		defer close(done)
		for {
			packet, err := ber.ReadPacket(server)
			if err != nil || len(packet.Children) < 2 {
				return
			}
			op := packet.Children[1]
			received <- int(op.Tag)
			if op.Tag == goldap.ApplicationExtendedRequest {
				response := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
				response.AppendChild(packet.Children[0])
				result := ber.Encode(ber.ClassApplication, ber.TypeConstructed, goldap.ApplicationExtendedResponse, nil, "")
				result.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, int64(goldap.LDAPResultProtocolError), ""))
				result.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
				result.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "no TLS here", ""))
				response.AppendChild(result)
				if _, err := server.Write(response.Bytes()); err != nil {
					return
				}
			}
		}
	})
	reader := Directory{Dial: func(context.Context, string) (net.Conn, error) { return client, nil }}
	err := reader.Probe(context.Background(), port.Connection{
		Directory: directoryAt(t, "ldap://dc.example.com"), Password: secret.New("pw"),
	})
	if detail(err) != "directory.ldap_unreachable" {
		t.Fatalf("a refused StartTLS: %v, want unreachable", err)
	}
	close(received)
	for tag := range received {
		if tag == goldap.ApplicationBindRequest {
			t.Fatal("the bind was sent in the clear")
		}
	}
}

func TestAnLdapBindWithoutAPasswordIsRefusedBeforeDialling(t *testing.T) {
	dialled := false
	reader := Directory{Dial: func(context.Context, string) (net.Conn, error) { dialled = true; return nil, errors.New("no") }}
	_, err := reader.Read(context.Background(), port.Connection{Directory: directoryAt(t, "ldaps://dc.example.com")})
	if detail(err) != "directory.ldap_refused" || dialled {
		t.Fatalf("an anonymous bind: %v, dialled %v", err, dialled)
	}
}

func TestAnUnwiredReaderDialsNothing(t *testing.T) {
	err := Directory{}.Probe(context.Background(), port.Connection{
		Directory: directoryAt(t, "ldaps://dc.example.com"), Password: secret.New("pw"),
	})
	if detail(err) != "directory.ldap_unwired" {
		t.Fatalf("unwired: %v", err)
	}
}

func TestTheDirectorysAnswerIsReadAsTheDomainReadsIt(t *testing.T) {
	d := directoryAt(t, "ldaps://dc.example.com")
	guid := string([]byte{0x8f, 0x1c, 0x00, 0xff})
	people := peopleOf([]*goldap.Entry{
		goldap.NewEntry("uid=lena,ou=people", map[string][]string{
			"objectGUID": {guid}, "mail": {" lena@example.com "}, "displayName": {"Lena"},
		}),
		goldap.NewEntry("uid=text,ou=people", map[string][]string{"objectGUID": {"abc-123"}}),
	}, d)
	if people[0].ID != "8f1c00ff" || people[0].Email != "lena@example.com" || people[0].Name != "Lena" || people[0].DN != "uid=lena,ou=people" {
		t.Fatalf("a person: %+v", people[0])
	}
	if people[1].ID != "abc-123" {
		t.Fatalf("a text identifier: %+v", people[1])
	}
	groups := groupsOf([]*goldap.Entry{goldap.NewEntry("cn=eng,ou=groups", map[string][]string{
		"objectGUID": {"g1"}, "cn": {"Engineering"}, "member": {"uid=lena,ou=people", "uid=text,ou=people"},
	})}, d)
	if groups[0].MemberNames != "g1" || groups[0].Name != "Engineering" || len(groups[0].MemberDNs) != 2 {
		t.Fatalf("a group: %+v", groups[0])
	}
}

func TestADirectorysRefusalIsCodedWithoutItsWords(t *testing.T) {
	refused := coded(goldap.NewError(goldap.LDAPResultInvalidCredentials, errors.New("80090308: LdapErr: DSID-0C09042A secret detail")))
	if detail(refused) != "directory.ldap_refused" {
		t.Fatalf("invalid credentials: %v", refused)
	}
	if cause := errors.Unwrap(refused); cause == nil || cause.Error() != "ldap result 49" {
		t.Fatalf("the server's words travel: %v", cause)
	}
	if detail(coded(goldap.NewError(goldap.ErrorNetwork, errors.New("reset")))) != "directory.ldap_unreachable" {
		t.Fatal("a broken connection is not unreachable")
	}
}
