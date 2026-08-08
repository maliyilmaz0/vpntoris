package nativehelper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativeengine"
	"vpntoris-tray/internal/netbackend"
	"vpntoris-tray/internal/runtimepaths"
)

func TestApplyNetworkJournalsAndRecover(t *testing.T) {
	root := t.TempDir()
	router := &recordingRouter{}
	journal, err := nativeengine.NewJournal(filepath.Join(root, "state"), journalOwner)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := nativeengine.NewManager(journal, netbackend.MutationBackend{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		EngineRoot: root,
		UserID:     -1,
		Router:     router,
		Manager:    manager,
		Paths: runtimepaths.Paths{
			RuntimeDirectory: filepath.Join(root, "r"),
			LogDirectory:     filepath.Join(root, "l"),
			StateDirectory:   filepath.Join(root, "state"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	current := &session{
		request: fortihelper.Request{Profile: "profile-office", Routes: []string{"10.38.0.0/16"}},
		command: nil,
	}
	if err := service.applyNetwork(current, "tun9"); err != nil {
		t.Fatal(err)
	}
	if len(router.added) != 1 {
		t.Fatalf("added = %#v", router.added)
	}
	if current.transaction == nil || current.transaction.State != nativeengine.TransactionActive {
		t.Fatalf("transaction = %#v", current.transaction)
	}
	listed, err := journal.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("journal list = %d %v", len(listed), err)
	}
	router2 := &recordingRouter{}
	manager2, err := nativeengine.NewManager(journal, netbackend.MutationBackend{Router: router2})
	if err != nil {
		t.Fatal(err)
	}
	service2, err := New(Config{
		EngineRoot: root,
		UserID:     -1,
		Router:     router2,
		Manager:    manager2,
		Paths: runtimepaths.Paths{
			RuntimeDirectory: filepath.Join(root, "r"),
			LogDirectory:     filepath.Join(root, "l"),
			StateDirectory:   filepath.Join(root, "state"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service2.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(router2.deleted) == 0 {
		t.Fatalf("expected recovery to delete owned routes, got %#v", router2.deleted)
	}
	listed, err = journal.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("journal after recover = %d %v", len(listed), err)
	}
	_ = os.RemoveAll(root)
}
func TestStopDeactivatesJournalTransaction(t *testing.T) {
	root := t.TempDir()
	router := &recordingRouter{}
	journal, err := nativeengine.NewJournal(filepath.Join(root, "state"), journalOwner)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := nativeengine.NewManager(journal, netbackend.MutationBackend{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		EngineRoot: root,
		UserID:     -1,
		Router:     router,
		Manager:    manager,
		Paths: runtimepaths.Paths{
			RuntimeDirectory: filepath.Join(root, "r"),
			LogDirectory:     filepath.Join(root, "l"),
			StateDirectory:   filepath.Join(root, "state"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	current := &session{
		request: fortihelper.Request{
			Profile:  "profile-office",
			Protocol: fortihelper.ProtocolOpenVPN,
			Routes:   []string{"10.1.0.0/16"},
		},
	}
	if err := service.applyNetwork(current, "tun1"); err != nil {
		t.Fatal(err)
	}
	service.sessions["profile-office"] = current
	current.interfaceName = "tun1"
	if response := service.Stop("profile-office"); response.State != "stopped" {
		t.Fatalf("stop = %#v", response)
	}
	if len(router.deleted) == 0 {
		t.Fatal("expected deactivate to delete routes")
	}
	listed, err := journal.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("journal after stop = %d %v", len(listed), err)
	}
}
