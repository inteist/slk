package main

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/service"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/slack-go/slack"
)

func TestOnConnectPresenceIncludesRecoveredStarredPeers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bodies := map[string]string{
			"/api/users.channelSections.list": `{"ok":true,"channel_sections":[{"channel_section_id":"ST","type":"stars","channel_ids_page":{"channel_ids":[],"count":0}}]}`,
			"/api/stars.list":                 `{"ok":true,"items":[{"type":"im","channel":"DNEW"},{"type":"im","channel":"DBOT"}]}`,
			"/api/users.getPresence":          `{"ok":true,"presence":"active"}`,
			"/api/dnd.info":                   `{"ok":true,"dnd_enabled":false}`,
			"/api/dnd.teamInfo":               `{"ok":true,"users":{}}`,
		}
		var asked []string
		client := slackclient.NewClient("xoxc-test", "d-cookie")
		client.HTTPClient().Transport = presenceSlackTransport(func(r *http.Request) (*http.Response, error) {
			body, ok := bodies[r.URL.Path]
			if !ok {
				t.Errorf("fake Slack got an unexpected request to %s", r.URL.Path)
				body = `{"ok":false,"error":"unexpected_path"}`
			}
			if r.URL.Path == "/api/dnd.teamInfo" {
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse DND request: %v", err)
				}
				asked = append(asked, strings.Split(r.Form.Get("users"), ",")...)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})
		wctx := &WorkspaceContext{
			TeamID:       "T1",
			UserID:       "USELF",
			Client:       client,
			SectionStore: service.NewSectionStore(),
			UserNames:    newUserNameStore(map[string]string{"UNEW": "New Peer", "UBOT": "Bot"}),
			Channels:     []sidebar.ChannelItem{{ID: "DEXISTING", Type: "dm", DMUserID: "UEXISTING"}},
		}
		wctx.MarkBotUser("UBOT")
		wctx.PeerStatus = newPeerStatusRefresher("T1", "USELF", nil, nil, client.GetDNDTeamInfo, nil, nil, nil)
		completed := false
		h := &rtmEventHandler{
			workspaceID: "T1",
			wsCtx:       wctx,
			program: sendFunc(func(msg tea.Msg) {
				if _, ok := msg.(ui.StatusChangeMsg); ok {
					completed = true
				}
			}),
		}
		h.resolveConversation = func(_ context.Context, id string) (*slack.Channel, error) {
			synctest.Wait()
			peer := map[string]string{"DNEW": "UNEW", "DBOT": "UBOT"}[id]
			if peer == "" {
				t.Fatalf("unexpected conversation %q", id)
			}
			return &slack.Channel{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: id, IsIM: true, User: peer}}}, nil
		}

		h.OnConnect()
		synctest.Wait()
		if !completed {
			t.Fatal("presence bootstrap did not complete")
		}
		if len(wctx.Channels) != 3 {
			t.Fatalf("recovered channels = %+v, want existing DM plus starred human and bot", wctx.Channels)
		}
		if want := []string{"UEXISTING", "UNEW"}; !reflect.DeepEqual(asked, want) {
			t.Fatalf("DND bootstrap peers = %v, want %v (no self or bot)", asked, want)
		}
	})
}

type presenceSlackTransport func(*http.Request) (*http.Response, error)

func (f presenceSlackTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestWorkspacePresenceIDs(t *testing.T) {
	wctx := &WorkspaceContext{
		UserID: "USELF",
		Channels: []sidebar.ChannelItem{
			{Type: "dm", DMUserID: "U1"},
			{Type: "channel"}, // not a DM — skipped
			{Type: "dm", DMUserID: "U2"},
			{Type: "dm", DMUserID: "U1"},     // duplicate peer — deduped
			{Type: "app", DMUserID: "UBOT"},  // app/bot DM — no presence dot
			{Type: "group_dm", DMUserID: ""}, // group DM — skipped
			{Type: "dm", DMUserID: ""},       // malformed — skipped
		},
	}

	got := workspacePresenceIDs(wctx)
	want := []string{"USELF", "U1", "U2"} // self first, then DM peers in order, deduped
	if len(got) != len(want) {
		t.Fatalf("workspacePresenceIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("workspacePresenceIDs()[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
}
