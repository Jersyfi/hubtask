// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestChecksumsSurviveTheRoundTrip(t *testing.T) {
	written := NewChecksums()
	for path, content := range map[string]string{
		ManifestName:           `{"format_version":1}`,
		DataName("containers"): "a\nb\n",
		DataName("work_items"): "c\n",
	} {
		if err := written.Add(path, digestOf(content)); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	var file bytes.Buffer
	if err := written.Encode(&file); err != nil {
		t.Fatalf("encode: %v", err)
	}

	read, err := ParseChecksums(&file)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(read.Paths()) != 3 {
		t.Fatalf("paths: %v", read.Paths())
	}
	if got, _ := read.Digest(ManifestName); got != digestOf(`{"format_version":1}`) {
		t.Fatalf("manifest digest lost: %s", got)
	}
}

// The shape is the one sha256sum reads, so that an operator with a directory and no Hubtask can
// check an archive with a tool that has been on every machine for thirty years.
func TestTheFileIsWhatSha256sumReads(t *testing.T) {
	checksums := NewChecksums()
	if err := checksums.Add(DataName("labels"), digestOf("x")); err != nil {
		t.Fatalf("add: %v", err)
	}

	var file bytes.Buffer
	if err := checksums.Encode(&file); err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := digestOf("x") + "  data/labels.jsonl\n"
	if file.String() != want {
		t.Fatalf("line %q, want %q", file.String(), want)
	}
}

// The same archive produces the same file twice: the order is by path, not by map iteration.
func TestTheFileIsStable(t *testing.T) {
	build := func() string {
		checksums := NewChecksums()
		for _, name := range []string{"work_items", "containers", "labels", "comments"} {
			if err := checksums.Add(DataName(name), digestOf(name)); err != nil {
				t.Fatalf("add: %v", err)
			}
		}
		var file bytes.Buffer
		if err := checksums.Encode(&file); err != nil {
			t.Fatalf("encode: %v", err)
		}
		return file.String()
	}

	first := build()
	for range 20 {
		if build() != first {
			t.Fatal("checksums.txt is not stable between runs")
		}
	}
	if !strings.HasPrefix(first, digestOf("comments")) {
		t.Fatalf("not sorted by path:\n%s", first)
	}
}

// A corrupted byte is found, and the failure says which member it was in.
func TestACorruptedByteIsFound(t *testing.T) {
	content := strings.Repeat("a line of an archive\n", 100)
	checksums := NewChecksums()
	if err := checksums.Add(DataName("comments"), digestOf(content)); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := checksums.Verify(DataName("comments"), strings.NewReader(content)); err != nil {
		t.Fatalf("an intact member failed: %v", err)
	}

	corrupted := []byte(content)
	corrupted[512] ^= 0x01
	err := checksums.Verify(DataName("comments"), bytes.NewReader(corrupted))
	if err == nil {
		t.Fatal("one flipped bit went unnoticed")
	}
	if got := detail(t, err); got != CodeChecksumMismatch {
		t.Fatalf("detail code %q", got)
	}
}

// A member the file does not list is not an archive with an extra file in it. It is an archive
// somebody has been editing.
func TestAMemberThatIsNotListedIsAFailure(t *testing.T) {
	checksums := NewChecksums()
	if err := checksums.Add(ManifestName, digestOf("{}")); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := checksums.Verify(DataName("smuggled"), strings.NewReader("anything"))
	if err == nil {
		t.Fatal("an unlisted member was accepted")
	}
	if got := detail(t, err); got != CodeChecksumMismatch {
		t.Fatalf("detail code %q", got)
	}
}

func TestAMemberWrittenTwiceWithDifferentContentIsADefect(t *testing.T) {
	checksums := NewChecksums()
	if err := checksums.Add(DataName("labels"), digestOf("a")); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := checksums.Add(DataName("labels"), digestOf("a")); err != nil {
		t.Fatalf("the same digest twice was refused: %v", err)
	}
	if err := checksums.Add(DataName("labels"), digestOf("b")); err == nil {
		t.Fatal("two different digests for one path were accepted")
	}
}

func TestAFileThatIsNotOneIsRefused(t *testing.T) {
	for name, content := range map[string]string{
		"empty":        "",
		"no separator": digestOf("x") + " data/labels.jsonl\n",
		"not a digest": "not-a-digest  data/labels.jsonl\n",
		"no path":      digestOf("x") + "  \n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseChecksums(strings.NewReader(content)); err == nil {
				t.Fatalf("%q was accepted", content)
			}
		})
	}
}

// Both numbers are wanted for every member and neither may cost a second pass over a stream that
// is on its way to somebody else's machine.
func TestTheCounterAnswersBothWithoutASecondPass(t *testing.T) {
	content := strings.Repeat("payload", 1000)
	counter := NewCounter()

	if _, err := counter.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if counter.Bytes() != int64(len(content)) {
		t.Fatalf("bytes %d, want %d", counter.Bytes(), len(content))
	}
	if counter.Digest() != digestOf(content) {
		t.Fatalf("digest %s, want %s", counter.Digest(), digestOf(content))
	}
}

func TestDigestAgreesWithTheCounter(t *testing.T) {
	content := "the same bytes, two ways"

	streamed, err := Digest(strings.NewReader(content))
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	counter := NewCounter()
	if _, err := counter.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if streamed != counter.Digest() {
		t.Fatalf("%s != %s", streamed, counter.Digest())
	}
}
