package main

import (
	"context"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestLogoutSession_NilSafety(t *testing.T) {
	// Should not panic on nil client or empty ID
	logoutSession(nil, "session-nil")

	container, err := sqlstore.New(context.Background(), "sqlite", "file:test_logout_safe?mode=memory&cache=shared&_pragma=foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	defer container.Close()

	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	cli := whatsmeow.NewClient(device, waLog.Noop)
	// device.ID is nil -> should safely return without error/panic
	logoutSession(cli, "session-no-id")
}

func TestLogoutSession_DisconnectedClient(t *testing.T) {
	container, err := sqlstore.New(context.Background(), "sqlite", "file:test_logout_disc?mode=memory&cache=shared&_pragma=foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	defer container.Close()

	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	id := types.NewADJID("628123456789", 0, 1)
	device.ID = &id

	cli := whatsmeow.NewClient(device, waLog.Noop)

	start := time.Now()
	// Should attempt connection, handle errors gracefully without panicking, and terminate
	logoutSession(cli, "session-disconnected")
	duration := time.Since(start)

	if duration > 40*time.Second {
		t.Fatalf("logoutSession took too long: %v", duration)
	}
}
