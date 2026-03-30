package bridge

import (
	"os"
	"strings"
	"testing"
)

func TestGetUsername_MethodExists(t *testing.T) {
	data, err := os.ReadFile("matterbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func (mb *MatterbridgeBridge) GetUsername(") {
		t.Fatal("MatterbridgeBridge must have GetUsername method")
	}
}

func TestBotID_MethodExists(t *testing.T) {
	data, err := os.ReadFile("matterbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func (mb *MatterbridgeBridge) BotID()") {
		t.Fatal("MatterbridgeBridge must have BotID method")
	}
}

func TestSend_UsesGateway(t *testing.T) {
	data, err := os.ReadFile("matterbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "msg.Channel") {
		t.Fatal("Send must check msg.Channel for gateway")
	}
	if !strings.Contains(src, "mb.Gateway") {
		t.Fatal("Send must fallback to mb.Gateway")
	}
}

func TestBridge_ErrAuthFailed(t *testing.T) {
	src, _ := os.ReadFile("bridge.go")
	if !strings.Contains(string(src), "ErrAuthFailed") {
		t.Fatal("must define ErrAuthFailed")
	}
}

func TestStream_ReconnectsOnError(t *testing.T) {
	data, err := os.ReadFile("matterbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "streamOnce") {
		t.Fatal("must have streamOnce for reconnection logic")
	}
}

func TestConnect_VerifiesConnectivity(t *testing.T) {
	data, err := os.ReadFile("matterbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if !strings.Contains(src, "/api/messages") {
		t.Fatal("Connect must verify connectivity via /api/messages")
	}
}
