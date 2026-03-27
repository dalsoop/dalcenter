package bridge

import (
	"os"
	"strings"
	"testing"
)

func TestGetUsername_MethodExists(t *testing.T) {
	data, err := os.ReadFile("mattermost.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func (m *MattermostBridge) GetUsername(") {
		t.Fatal("MattermostBridge must have GetUsername method")
	}
}
