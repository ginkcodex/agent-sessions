package app

import (
	"github.com/ginkcode/agent-sessions/internal/rpc"
	"testing"
)

func TestGitHistoryConnectionCapability(t *testing.T) {
	a := NewApp()
	a.backend = &historyRecorder{}
	if !a.ConnectionState().Capabilities.GitHistory {
		t.Fatal("local history not advertised")
	}
	a.backend = &gitBranchesRecorder{}
	if a.ConnectionState().Capabilities.GitHistory {
		t.Fatal("unsupported local history advertised")
	}
	a.backend = &historyRecorder{}
	a.conn.wantHost = "box"
	a.conn.localActive = false
	a.conn.client = &gitBranchesRecorder{}
	a.conn.state = ConnectionState{Phase: ConnConnected, Host: "box", Capabilities: rpc.Capabilities{GitHistory: false}}
	if a.ConnectionState().Capabilities.GitHistory {
		t.Fatal("local support leaked to old remote")
	}
	a.conn.state.Capabilities.GitHistory = true
	if !a.ConnectionState().Capabilities.GitHistory {
		t.Fatal("remote support lost")
	}
	a.conn.client = nil
	a.conn.state = ConnectionState{Phase: ConnReconnecting, Host: "box"}
	if a.ConnectionState().Capabilities.GitHistory {
		t.Fatal("local support leaked to offline remote")
	}
}
