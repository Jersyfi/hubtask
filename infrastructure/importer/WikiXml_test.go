// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
)

const wikiEntities = `<?xml version="1.0" encoding="UTF-8"?>
<hibernate-generic datetime="2026-10-30 10:00:00">
<object class="Space" package="com.example"><id name="id">1</id><property name="name"><![CDATA[Team Handbook]]></property></object>
<object class="ConfluenceUserImpl" package="com.example"><id name="key"><![CDATA[u-ada]]></id><property name="name"><![CDATA[ada]]></property></object>
<object class="ConfluenceUserImpl" package="com.example"><id name="key"><![CDATA[u-bob]]></id><property name="name"><![CDATA[bob]]></property></object>
<object class="Page" package="com.example"><id name="id">100</id>
 <property name="title"><![CDATA[Onboarding]]></property><property name="version">3</property>
 <property name="creationDate">2026-03-01 09:00:00.000</property><property name="lastModificationDate">2026-03-03 10:00:00.000</property>
 <property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Page" package="com.example"><id name="id">101</id>
 <property name="title"><![CDATA[Onboarding draft]]></property><property name="version">1</property>
 <property name="originalVersion" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="versionComment"><![CDATA[first cut]]></property>
 <property name="lastModificationDate">2026-03-01 09:00:00.000</property><property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Page" package="com.example"><id name="id">102</id>
 <property name="title"><![CDATA[Onboarding]]></property><property name="version">2</property>
 <property name="originalVersion" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="lastModificationDate">2026-03-02 09:00:00.000</property><property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Page" package="com.example"><id name="id">200</id>
 <property name="title"><![CDATA[Tools]]></property><property name="version">1</property>
 <property name="parent" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="lastModificationDate">2026-03-04 10:00:00.000</property><property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Page" package="com.example"><id name="id">300</id>
 <property name="title"><![CDATA[Half written]]></property><property name="contentStatus"><![CDATA[draft]]></property></object>
<object class="BodyContent" package="com.example"><id name="id">9100</id>
 <property name="body"><![CDATA[<h2>First day</h2><p>Read <ac:link><ri:page ri:content-title="Tools" /><ac:plain-text-link-body><![CDATA[the tools]]]]><![CDATA[></ac:plain-text-link-body></ac:link> first.</p><ac:structured-macro ac:name="code"><ac:parameter ac:name="language">sh</ac:parameter><ac:plain-text-body><![CDATA[make setup]]]]><![CDATA[></ac:plain-text-body></ac:structured-macro><ac:structured-macro ac:name="info"><ac:rich-text-body><p>Ask <strong>IT</strong>.</p></ac:rich-text-body></ac:structured-macro><p><ac:image><ri:attachment ri:filename="map.png" /></ac:image></p>]]></property>
 <property name="content" class="Page" package="com.example"><id name="id">100</id></property></object>
<object class="BodyContent" package="com.example"><id name="id">9101</id>
 <property name="body"><![CDATA[<p>Just a start.</p>]]></property>
 <property name="content" class="Page" package="com.example"><id name="id">101</id></property></object>
<object class="BodyContent" package="com.example"><id name="id">9102</id>
 <property name="body"><![CDATA[<p>A start and a plan.</p>]]></property>
 <property name="content" class="Page" package="com.example"><id name="id">102</id></property></object>
<object class="BodyContent" package="com.example"><id name="id">9200</id>
 <property name="body"><![CDATA[<ac:task-list><ac:task><ac:task-status>complete</ac:task-status><ac:task-body>Install</ac:task-body></ac:task></ac:task-list>]]></property>
 <property name="content" class="Page" package="com.example"><id name="id">200</id></property></object>
<object class="Comment" package="com.example"><id name="id">500</id>
 <property name="containerContent" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="creator" class="ConfluenceUserImpl" package="com.example"><id name="key"><![CDATA[u-ada]]></id></property>
 <property name="creationDate">2026-03-03 11:00:00.000</property><property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Comment" package="com.example"><id name="id">501</id>
 <property name="containerContent" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="parent" class="Comment" package="com.example"><id name="id">500</id></property>
 <property name="creator" class="ConfluenceUserImpl" package="com.example"><id name="key"><![CDATA[u-bob]]></id></property>
 <property name="creationDate">2026-03-03 12:00:00.000</property><property name="contentStatus"><![CDATA[current]]></property></object>
<object class="Comment" package="com.example"><id name="id">502</id>
 <property name="containerContent" class="Page" package="com.example"><id name="id">100</id></property>
 <property name="creationDate">2026-03-03 13:00:00.000</property><property name="contentStatus"><![CDATA[deleted]]></property></object>
<object class="BodyContent" package="com.example"><id name="id">9500</id>
 <property name="body"><![CDATA[<p>Which <em>laptop</em>?</p>]]></property>
 <property name="content" class="Comment" package="com.example"><id name="id">500</id></property></object>
<object class="BodyContent" package="com.example"><id name="id">9501</id>
 <property name="body"><![CDATA[<p>The light one.</p>]]></property>
 <property name="content" class="Comment" package="com.example"><id name="id">501</id></property></object>
<object class="ContentProperty" package="com.example"><id name="id">600</id>
 <property name="name"><![CDATA[inline-original-selection]]></property><property name="stringValue"><![CDATA[First day]]></property>
 <property name="content" class="Comment" package="com.example"><id name="id">500</id></property></object>
<object class="Attachment" package="com.example"><id name="id">700</id>
 <property name="title"><![CDATA[map.png]]></property>
 <property name="containerContent" class="Page" package="com.example"><id name="id">100</id></property></object>
<object class="Attachment" package="com.example"><id name="id">701</id>
 <property name="title"><![CDATA[brief.pdf]]></property>
 <property name="containerContent" class="Page" package="com.example"><id name="id">100</id></property></object>
</hibernate-generic>`

// wikiXMLZip is a space's XML export: its entities, two versions of a map and a brief.
func wikiXMLZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range []struct{ name, content string }{
		{"entities.xml", wikiEntities},
		{"exportDescriptor.properties", "spaceKey=TEAM"},
		{"attachments/100/700/1", "\x89PNG\r\n\x1a\nold map"},
		{"attachments/100/700/2", "\x89PNG\r\n\x1a\nnew map"},
		{"attachments/100/701/1", "%PDF-1.4 the brief"},
		{"attachments/999/702/1", "a page not exported"},
	} {
		member, err := writer.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func versionsOf(records []archive.Record, page any) []map[string]any {
	var out []map[string]any
	for _, record := range records {
		if record.Data["page_id"] == page {
			out = append(out, record.Data)
		}
	}
	return out
}

// UC-IMP-06 checks 1 and 2: the XML export's pages land in their tree with their content, and
// each page's earlier versions become its history, oldest first, the current content last.
func TestASpacesXMLExportCarriesThePagesWithTheirHistory(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiXMLZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	pages := result.Records["pages"]
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want the two current pages without the draft", len(pages))
	}
	onboarding, tools := titled(pages, "Onboarding"), titled(pages, "Tools")
	if onboarding == nil || tools == nil || tools["parent_id"] != onboarding["id"] {
		t.Fatalf("tree = %v / %v", onboarding, tools)
	}
	if onboarding["created_at"] != "2026-03-01T09:00:00Z" || onboarding["updated_at"] != "2026-03-03T10:00:00Z" {
		t.Errorf("dates = %v, %v", onboarding["created_at"], onboarding["updated_at"])
	}
	body := onboarding["body"].(string)
	for _, want := range []string{"## First day", "[the tools](hubtask:page/" + tools["id"].(string) + ")", "```\nmake setup\n```", "Ask **IT**."} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if !strings.Contains(tools["body"].(string), "[x] Install") {
		t.Errorf("tools = %q", tools["body"])
	}
	versions := versionsOf(result.Records["page_versions"], onboarding["id"])
	if len(versions) != 3 {
		t.Fatalf("versions = %v", versions)
	}
	if versions[0]["seq"] != 1 || versions[0]["title"] != "Onboarding draft" || versions[0]["body"] != "Just a start." ||
		versions[0]["name"] != "first cut" || versions[0]["created_at"] != "2026-03-01T09:00:00Z" {
		t.Errorf("first version = %v", versions[0])
	}
	if versions[1]["body"] != "A start and a plan." || versions[2]["seq"] != 3 || versions[2]["body"] != body {
		t.Errorf("later versions = %v / %v", versions[1], versions[2])
	}
}

// UC-IMP-06 check 3: comments and replies land on their page with their author named and their
// moment, an inline comment with the words it was written about; a removed one is not carried.
func TestASpacesXMLExportCarriesTheComments(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiXMLZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	onboarding := titled(result.Records["pages"], "Onboarding")
	comments := result.Records["page_comments"]
	if len(comments) != 2 {
		t.Fatalf("comments = %v", comments)
	}
	thread, reply := comments[0].Data, comments[1].Data
	if thread["page_id"] != onboarding["id"] || thread["body"] != "ada: Which *laptop*?" || thread["quote"] != "First day" ||
		thread["parent_id"] != nil || thread["created_at"] != "2026-03-03T11:00:00Z" || thread["author_id"] != actor.String() {
		t.Errorf("thread = %v", thread)
	}
	if reply["parent_id"] != thread["id"] || reply["body"] != "bob: The light one." || reply["quote"] != "" {
		t.Errorf("reply = %v", reply)
	}
}

// UC-IMP-06 check 4: the attachments' newest versions become the page's files, a picture drawn
// where it stood; an earlier version and a file of a page not exported are counted.
func TestASpacesXMLExportCarriesTheNewestAttachments(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiXMLZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	media := map[string]string{}
	for _, record := range result.Records["media_objects"] {
		media[record.Data["file_name"].(string)] = record.Data["id"].(string)
	}
	if len(media) != 2 || media["map.png"] == "" || media["brief.pdf"] == "" {
		t.Fatalf("media = %v", media)
	}
	if got := string(result.Media[result.Records["media_objects"][0].Blobs[0].Digest]); !strings.Contains(got, "new map") && !strings.Contains(got, "brief") {
		t.Errorf("bytes = %q", got)
	}
	body := titled(result.Records["pages"], "Onboarding")["body"].(string)
	if !strings.HasSuffix(body, "![](hubtask:media/"+media["map.png"]+")\n\n![](hubtask:media/"+media["brief.pdf"]+")") {
		t.Errorf("body = %q", body)
	}
	if result.Unmapped["attachments"] != 2 {
		t.Errorf("unmapped = %v, want the earlier map and the file of a page not exported", result.Unmapped)
	}
	again, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiXMLZip(t)))
	if err != nil || titled(again.Records["pages"], "Onboarding")["id"] != titled(result.Records["pages"], "Onboarding")["id"] {
		t.Error("the same export lands on the same pages")
	}
}
