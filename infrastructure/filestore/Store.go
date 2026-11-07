// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filestore speaks to the file stores a person connects (UC-INT-17), through the guarded
// client (rule 6): a Nextcloud's login flow v2, its WebDAV folders and its OCS share links, and
// Microsoft Graph's OneDrive and SharePoint folders. It reads names, sizes and addresses - never a
// file's content.
package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/filestore"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// TargetClass labels the calls for the outbound duration histogram.
const TargetClass = "file_store"

// appName is what a Nextcloud names the app password under *Devices & sessions*.
const appName = "Hubtask"

const graph = "https://graph.microsoft.com/v1.0"

// Store is every store's door.
type Store struct {
	Client httpport.Port
}

var _ port.Store = Store{}

func unreachable(cause error) error {
	return shared.ErrUnavailable.WithDetail("filestore.store_unreachable").WithCause(cause)
}

// refusal codes a store's answer.
func refusal(status int) error {
	switch {
	case status/100 == 2 || status == http.StatusMultiStatus:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return shared.ErrForbidden.WithDetail("filestore.grant_revoked").
			WithCause(errors.New("file store: the store answered " + http.StatusText(status)))
	case status == http.StatusNotFound:
		return shared.ErrNotFound.WithDetail("filestore.item_unknown")
	}
	return unreachable(errors.New("file store: the store answered " + http.StatusText(status)))
}

func (s Store) call(ctx context.Context, method, address string, header map[string][]string, body []byte) (httpport.Response, error) {
	if s.Client == nil {
		return httpport.Response{}, unreachable(errors.New("file store: no outbound client is wired"))
	}
	if header == nil {
		header = map[string][]string{}
	}
	header["User-Agent"] = []string{appName}
	response, err := s.Client.Do(ctx, httpport.Request{Method: method, URL: address, Header: header, Body: body, TargetClass: TargetClass})
	if err != nil {
		return httpport.Response{}, unreachable(err)
	}
	return response, nil
}

// sameOrigin reports whether an address a Nextcloud answered stays on its own https origin, so a
// store cannot send the poll or the person anywhere else.
func sameOrigin(server, address string) bool {
	s, err1 := url.Parse(server)
	a, err2 := url.Parse(address)
	return err1 == nil && err2 == nil && a.Scheme == "https" && strings.EqualFold(a.Host, s.Host) && a.User == nil
}

func (s Store) BeginLogin(ctx context.Context, server string) (port.Login, error) {
	response, err := s.call(ctx, http.MethodPost, server+"/index.php/login/v2", map[string][]string{"Accept": {"application/json"}}, nil)
	if err != nil {
		return port.Login{}, err
	}
	if response.Status/100 != 2 {
		return port.Login{}, shared.ErrValidation.WithDetail("filestore.not_a_nextcloud")
	}
	var answer struct {
		Poll struct {
			Token    string `json:"token"`
			Endpoint string `json:"endpoint"`
		} `json:"poll"`
		Login string `json:"login"`
	}
	if json.Unmarshal(response.Body, &answer) != nil || answer.Poll.Token == "" ||
		!sameOrigin(server, answer.Poll.Endpoint) || !sameOrigin(server, answer.Login) {
		return port.Login{}, shared.ErrValidation.WithDetail("filestore.not_a_nextcloud")
	}
	return port.Login{LoginURL: answer.Login, PollEndpoint: answer.Poll.Endpoint, PollToken: secret.New(answer.Poll.Token)}, nil
}

func (s Store) PollLogin(ctx context.Context, endpoint string, token secret.Secret) (port.Grant, bool, error) {
	body := []byte(url.Values{"token": {token.Reveal()}}.Encode())
	response, err := s.call(ctx, http.MethodPost, endpoint, map[string][]string{
		"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"},
	}, body)
	if err != nil {
		return port.Grant{}, false, err
	}
	if response.Status == http.StatusNotFound {
		return port.Grant{}, false, nil
	}
	if response.Status/100 != 2 {
		return port.Grant{}, false, unreachable(errors.New("file store: the login poll answered " + http.StatusText(response.Status)))
	}
	var answer struct {
		Server      string `json:"server"`
		LoginName   string `json:"loginName"`
		AppPassword string `json:"appPassword"`
	}
	if json.Unmarshal(response.Body, &answer) != nil || answer.LoginName == "" || answer.AppPassword == "" {
		return port.Grant{}, false, unreachable(errors.New("file store: the login poll answered no grant"))
	}
	return port.Grant{Server: answer.Server, Login: answer.LoginName, AppPassword: secret.New(answer.AppPassword)}, true, nil
}

func basic(access port.Access) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(access.Login+":"+access.Secret.Reveal()))
}

func (s Store) Browse(ctx context.Context, access port.Access, folder string) ([]filelink.Item, error) {
	if access.Kind == filelink.ConnectMicrosoft {
		return s.browseGraph(ctx, access, folder)
	}
	return s.browseNextcloud(ctx, access, folder)
}

// davPath is a reference's address under the person's own files, each segment escaped.
func davPath(access port.Access, ref string) string {
	segments := []string{url.PathEscape(access.Login)}
	for _, segment := range strings.Split(strings.Trim(ref, "/"), "/") {
		if segment != "" {
			segments = append(segments, url.PathEscape(segment))
		}
	}
	return "/remote.php/dav/files/" + strings.Join(segments, "/")
}

const propfind = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
 <d:prop><d:displayname/><d:resourcetype/><d:getcontentlength/><d:getlastmodified/><oc:fileid/><oc:size/></d:prop>
</d:propfind>`

type multistatus struct {
	Responses []struct {
		Href  string `xml:"href"`
		Props []struct {
			Status string `xml:"status"`
			Prop   struct {
				DisplayName  string `xml:"displayname"`
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
				Length   string `xml:"getcontentlength"`
				Size     string `xml:"size"`
				Modified string `xml:"getlastmodified"`
				FileID   string `xml:"fileid"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func (s Store) browseNextcloud(ctx context.Context, access port.Access, folder string) ([]filelink.Item, error) {
	base, err := url.Parse(access.Server)
	if err != nil {
		return nil, unreachable(err)
	}
	root := base.Path + davPath(access, "")
	self := base.Path + davPath(access, folder)
	response, err := s.call(ctx, "PROPFIND", access.Server+davPath(access, folder), map[string][]string{
		"Authorization": {basic(access)}, "Depth": {"1"}, "Content-Type": {"application/xml"},
	}, []byte(propfind))
	if err != nil {
		return nil, err
	}
	if err := refusal(response.Status); err != nil {
		return nil, err
	}
	var answer multistatus
	if err := xml.NewDecoder(bytes.NewReader(response.Body)).Decode(&answer); err != nil {
		return nil, unreachable(err)
	}
	items := []filelink.Item{}
	for _, r := range answer.Responses {
		href, err := url.PathUnescape(r.Href)
		if err != nil || !strings.HasPrefix(href, root) {
			continue
		}
		if strings.TrimRight(href, "/") == strings.TrimRight(mustUnescape(self), "/") {
			continue
		}
		ref := "/" + strings.Trim(strings.TrimPrefix(href, root), "/")
		item := filelink.Item{Ref: ref, Name: path.Base(ref), Provider: filelink.Nextcloud}
		for _, ps := range r.Props {
			if !strings.Contains(ps.Status, " 200 ") {
				continue
			}
			p := ps.Prop
			if p.DisplayName != "" {
				item.Name = p.DisplayName
			}
			item.Folder = p.ResourceType.Collection != nil
			size := p.Length
			if size == "" {
				size = p.Size
			}
			item.Size, _ = strconv.ParseInt(size, 10, 64)
			if at, err := http.ParseTime(p.Modified); err == nil {
				item.Modified = at.UTC()
			}
			if p.FileID != "" {
				item.Address = access.Server + "/index.php/f/" + url.PathEscape(p.FileID)
			}
		}
		if item.Address == "" {
			item.Address = access.Server + "/index.php/apps/files/?dir=" + url.QueryEscape(path.Dir(ref))
		}
		items = append(items, item)
		if len(items) == filelink.MaxFolderItems {
			break
		}
	}
	return items, nil
}

func mustUnescape(p string) string {
	if out, err := url.PathUnescape(p); err == nil {
		return out
	}
	return p
}

func (s Store) Share(ctx context.Context, access port.Access, ref string) (string, error) {
	if access.Kind != filelink.ConnectNextcloud {
		return "", shared.ErrValidation.WithDetail("filestore.share_not_offered")
	}
	body := []byte(url.Values{"path": {"/" + strings.Trim(ref, "/")}, "shareType": {"3"}, "permissions": {"1"}}.Encode())
	response, err := s.call(ctx, http.MethodPost, access.Server+"/ocs/v2.php/apps/files_sharing/api/v1/shares", map[string][]string{
		"Authorization": {basic(access)}, "OCS-APIRequest": {"true"}, "Accept": {"application/json"},
		"Content-Type": {"application/x-www-form-urlencoded"},
	}, body)
	if err != nil {
		return "", err
	}
	if err := refusal(response.Status); err != nil {
		return "", err
	}
	var answer struct {
		OCS struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"ocs"`
	}
	if json.Unmarshal(response.Body, &answer) != nil || !sameOrigin(access.Server, answer.OCS.Data.URL) {
		return "", unreachable(errors.New("file store: the share answered no link"))
	}
	return answer.OCS.Data.URL, nil
}

func (s Store) Revoke(ctx context.Context, access port.Access) error {
	if access.Kind != filelink.ConnectNextcloud {
		return nil
	}
	_, err := s.call(ctx, http.MethodDelete, access.Server+"/ocs/v2.php/core/apppassword", map[string][]string{
		"Authorization": {basic(access)}, "OCS-APIRequest": {"true"},
	}, nil)
	return err
}

// graphRef is a Graph folder's reference: `site:<id>` for a SharePoint site's libraries, or
// `drive:<drive>:<item>` for a folder, `me` the person's own OneDrive.
func graphRef(drive, item string) string { return "drive:" + drive + ":" + item }

type graphPage struct {
	Value []struct {
		ID           string          `json:"id"`
		Name         string          `json:"name"`
		DisplayName  string          `json:"displayName"`
		WebURL       string          `json:"webUrl"`
		Size         int64           `json:"size"`
		Folder       json.RawMessage `json:"folder"`
		LastModified time.Time       `json:"lastModifiedDateTime"`
	} `json:"value"`
	Next string `json:"@odata.nextLink"`
}

func (s Store) graphGet(ctx context.Context, access port.Access, address string, page *graphPage) error {
	response, err := s.call(ctx, http.MethodGet, address, map[string][]string{
		"Authorization": {"Bearer " + access.Secret.Reveal()}, "Accept": {"application/json"},
	}, nil)
	if err != nil {
		return err
	}
	if err := refusal(response.Status); err != nil {
		return err
	}
	if err := json.Unmarshal(response.Body, page); err != nil {
		return unreachable(err)
	}
	return nil
}

func (s Store) browseGraph(ctx context.Context, access port.Access, folder string) ([]filelink.Item, error) {
	items := []filelink.Item{}
	var address string
	drive := ""
	switch {
	case folder == "" || folder == "/":
		items = append(items, filelink.Item{Ref: graphRef("me", "root"), Name: "OneDrive", Folder: true, Provider: filelink.OneDrive})
		address = graph + "/me/followedSites?$select=id,displayName,webUrl"
	case strings.HasPrefix(folder, "site:"):
		address = graph + "/sites/" + url.PathEscape(strings.TrimPrefix(folder, "site:")) + "/drives?$select=id,name,webUrl"
	case strings.HasPrefix(folder, "drive:"):
		parts := strings.SplitN(strings.TrimPrefix(folder, "drive:"), ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, shared.ErrValidation.WithDetail("filestore.folder_invalid")
		}
		drive = parts[0]
		base := graph + "/drives/" + url.PathEscape(drive)
		if drive == "me" {
			base = graph + "/me/drive"
		}
		address = base + "/items/" + url.PathEscape(parts[1]) + "/children?$top=200&$select=id,name,size,webUrl,folder,lastModifiedDateTime"
	default:
		return nil, shared.ErrValidation.WithDetail("filestore.folder_invalid")
	}
	for address != "" && len(items) < filelink.MaxFolderItems {
		var page graphPage
		if err := s.graphGet(ctx, access, address, &page); err != nil {
			return nil, err
		}
		for _, v := range page.Value {
			item := filelink.Item{Name: v.Name, Address: v.WebURL, Size: v.Size, Modified: v.LastModified}
			switch {
			case folder == "" || folder == "/":
				item.Ref, item.Name, item.Folder, item.Provider = "site:"+v.ID, v.DisplayName, true, filelink.SharePoint
			case strings.HasPrefix(folder, "site:"):
				item.Ref, item.Folder, item.Provider = graphRef(v.ID, "root"), true, filelink.SharePoint
			default:
				item.Ref, item.Folder = graphRef(drive, v.ID), len(v.Folder) > 0 && string(v.Folder) != "null"
				if u, err := url.Parse(v.WebURL); err == nil {
					item.Provider = filelink.Detect(u)
				}
			}
			items = append(items, item)
			if len(items) == filelink.MaxFolderItems {
				break
			}
		}
		address = ""
		if strings.HasPrefix(page.Next, graph+"/") {
			address = page.Next
		}
	}
	return items, nil
}
