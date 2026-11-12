package engine

import (
	"context"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

func TestServiceChatLiveOverlay(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentCodex, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) { testServiceChatLiveOverlay(t, agent) })
	}
}

func testServiceChatLiveOverlay(t *testing.T, agent model.AgentID) {
	owners := &chatOwners{m: map[string]string{}}
	sessionID := "chat-session"
	if agent == model.AgentOpenCode {
		sessionID = "ses_live_session"
	}
	ref := model.SessionRef{Agent: agent, ID: sessionID}
	catalog := scan.NewCatalog()
	chatMeta := model.SessionMeta{Ref: ref, CWD: "/tmp/chat-project", Title: "Chat session"}
	providerLive := model.SessionMeta{Ref: model.SessionRef{Agent: agent, ID: "cli-session"}, CWD: chatMeta.CWD, Live: true, LiveStatus: "working"}
	archived := model.SessionMeta{Ref: model.SessionRef{Agent: agent, ID: "archived"}, CWD: chatMeta.CWD, Archived: true}
	catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{chatMeta, providerLive, archived}})
	svc := NewService(catalog, nil)
	svc.SetDataDir(t.TempDir())
	svc.chatOwners = owners
	check := func(want int, owned bool) {
		t.Helper()
		got, err := svc.ListSessions("", FilterOpts{LiveOnly: true}, SortOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != want {
			t.Fatalf("live sessions=%+v want=%d", got, want)
		}
		counts, err := svc.AgentCounts(FilterOpts{LiveOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		if counts[string(agent)] != want {
			t.Fatalf("live counts=%v want=%d", counts, want)
		}
		groups, err := svc.ListGroups(GroupModeDirAgent, FilterOpts{LiveOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].SessionCount != want {
			t.Fatalf("live groups=%+v want=%d", groups, want)
		}
		meta, err := svc.GetSessionMeta(ref)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Live != owned {
			t.Fatalf("chat meta.Live=%v want=%v", meta.Live, owned)
		}
		raw, _ := catalog.Get(ref)
		if raw.Live {
			t.Fatal("chat overlay leaked into scanner catalog")
		}
		live, err := svc.GetSessionMeta(providerLive.Ref)
		if err != nil {
			t.Fatal(err)
		}
		if !live.Live || live.LiveStatus != "working" {
			t.Fatalf("provider live badge lost: %+v", live)
		}
	}
	check(1, false)
	if err := owners.claim(ref, "chat-1"); err != nil {
		t.Fatal(err)
	}
	if err := owners.claim(archived.Ref, "archived-chat"); err != nil {
		t.Fatal(err)
	}
	check(2, true)
	// Unchanged process detection or replacement by another scan cannot erase it.
	catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{chatMeta}})
	check(2, true)
	for _, filter := range []FilterOpts{{LiveOnly: true, Agent: "claude-code"}, {LiveOnly: true, Query: "does not match"}, {LiveOnly: true, Path: "/other/project"}} {
		got, err := svc.ListSessions("", filter, SortOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("chat bypassed filter %+v: %+v", filter, got)
		}
	}
	owners.release("chat-1")
	check(1, false)
}

func TestEngineChatLiveLifecycleNotifications(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentCodex, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) { testEngineChatLiveLifecycleNotifications(t, agent) })
	}
}

func testEngineChatLiveLifecycleNotifications(t *testing.T, agent model.AgentID) {
	for _, ending := range []string{"close", "crash"} {
		t.Run(ending, func(t *testing.T) {
			svc := NewService(scan.NewCatalog(), nil)
			svc.SetDataDir(t.TempDir())
			e := NewEngine(WithService(svc), WithChatLaunch(func(_ context.Context, _ model.AgentID, dir string) (acp.Command, error) {
				if agent == model.AgentOpenCode {
					return acptest.Command(dir, "ACPTEST_NO_RESUME=1"), nil
				}
				return acptest.Command(dir), nil
			}))
			e.chats.owners = &chatOwners{m: map[string]string{}}
			svc.chatOwners = e.chats.owners
			timers := &fakeTimers{}
			record := &recorder{}
			e.events = NewCatalogBusWith(record, timers.afterFunc, 0)
			t.Cleanup(e.events.Stop)
			t.Cleanup(e.chats.CloseAll)
			sessionID := "live-session"
			if agent == model.AgentOpenCode {
				sessionID = "ses_live_session"
			}
			ref := model.SessionRef{Agent: agent, ID: sessionID}
			svc.catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: ref, CWD: t.TempDir()}}})
			st, err := e.chats.Start(chatCtx(t), ChatStartRequest{Agent: agent, Dir: t.TempDir(), Resume: &ref})
			if err != nil {
				t.Fatal(err)
			}
			if st.Working {
				t.Fatal("expected an idle open chat")
			}
			got, err := e.ListSessions(context.Background(), "", FilterOpts{LiveOnly: true}, SortOpts{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("idle chat not Live: %+v", got)
			}
			if timers.fire() != 1 || len(record.get()) != 1 || !record.get()[0].GroupsDirty {
				t.Fatal("chat start did not invalidate catalog views")
			}
			if ending == "close" {
				if err := e.chats.Close(st.ChatID); err != nil {
					t.Fatal(err)
				}
			} else {
				// The fake adapter exits while a turn is running.
				ch, err := e.chats.get(st.ChatID)
				if err != nil {
					t.Fatal(err)
				}
				if err := e.chats.Prompt(st.ChatID, "crash", nil); err != nil {
					t.Fatal(err)
				}
				// Join the watcher without sending a second request to the adapter.
				select {
				case <-ch.done:
				case <-chatCtx(t).Done():
					t.Fatal("chat did not exit")
				}
			}
			got, err = e.ListSessions(context.Background(), "", FilterOpts{LiveOnly: true}, SortOpts{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 0 {
				t.Fatalf("ended chat still Live: %+v", got)
			}
			if timers.fire() != 1 || len(record.get()) != 2 {
				t.Fatal("chat exit did not invalidate catalog views")
			}
			// A failed resume must release its reserved ownership, too.
			failed := model.SessionRef{Agent: agent, ID: "in-use"}
			if _, err := e.chats.Start(chatCtx(t), ChatStartRequest{Agent: agent, Dir: t.TempDir(), Resume: &failed}); err == nil {
				t.Fatal("expected failed resume")
			}
			if _, owned := e.chats.Owned(failed); owned {
				t.Fatalf("failed resume still owns %+v", failed)
			}
		})
	}
}

func TestOpenCodeNewChatBecomesLiveWhenCatalogRecordArrives(t *testing.T) {
	f := newChatFixture(t, "ACPTEST_NO_RESUME=1")
	svc := NewService(scan.NewCatalog(), nil)
	svc.SetDataDir(t.TempDir())
	svc.chatOwners = f.m.owners
	st, err := f.m.Start(chatCtx(t), ChatStartRequest{Agent: model.AgentOpenCode, Dir: f.dir})
	if err != nil {
		t.Fatal(err)
	}
	if st.Session.Agent != model.AgentOpenCode || st.Session.ID == "" {
		t.Fatalf("new OpenCode identity=%+v", st.Session)
	}
	list := func() []model.SessionMeta {
		t.Helper()
		got, err := svc.ListSessions("", FilterOpts{LiveOnly: true}, SortOpts{})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := list(); len(got) != 0 {
		t.Fatalf("unsaved chat created a phantom catalog session: %+v", got)
	}
	// The watcher/provider discovers the saved session while the chat stays open.
	meta := model.SessionMeta{Ref: st.Session, CWD: f.dir, Title: "OpenCode chat"}
	svc.catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{meta}})
	if got := list(); len(got) != 1 || got[0].Ref != st.Session || !got[0].Live {
		t.Fatalf("saved OpenCode chat missing from Live: %+v", got)
	}
	// Scanner refreshes leave the chat-owned badge intact.
	svc.catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{meta}})
	if got := list(); len(got) != 1 {
		t.Fatalf("scan lost chat liveness: %+v", got)
	}
	if err := f.m.Close(st.ChatID); err != nil {
		t.Fatal(err)
	}
	if got := list(); len(got) != 0 {
		t.Fatalf("closed OpenCode chat still live: %+v", got)
	}
}
