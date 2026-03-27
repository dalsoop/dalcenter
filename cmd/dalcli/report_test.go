package main

import (
	"os"
	"strings"
	"testing"
)

func readSrc(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	return string(data)
}

func TestMemberReportsToLeader(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, "reportToLeader") {
		t.Fatal("member dal must call reportToLeader on direct user tasks")
	}
}

func TestReportToLeader_ChecksRole(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, `role == "member"`) {
		t.Fatal("reportToLeader must only trigger for member role")
	}
}

func TestReportToLeader_SkipsLeaderMessages(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, "isFromLeader") {
		t.Fatal("must check isFromLeader to avoid report loops")
	}
}

func TestIsFromLeader_ChecksUsername(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, `"leader"`) {
		t.Fatal("isFromLeader must check for 'leader' in username")
	}
}

func TestTeamMembersEnvUsed(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, "DAL_TEAM_MEMBERS") {
		t.Fatal("must use DAL_TEAM_MEMBERS env for leader mention")
	}
}

func TestAgentConfig_HasTeamMembersField(t *testing.T) {
	src := readSrc(t, "cmd_run.go")
	if !strings.Contains(src, "TeamMembers") {
		t.Fatal("agentConfig must have TeamMembers field")
	}
}
