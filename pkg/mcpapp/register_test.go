package mcpapp_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

type echoArgs struct {
	Text string `json:"text"`
}

type echoResult struct {
	Text string `json:"text"`
}

// TestRegisterServesSDDToolsBesideHostTools composes a host-supplied server
// carrying SDD's tools and a host tool, and drives both through one
// connection (d-tac-d3f).
func TestRegisterServesSDDToolsBesideHostTools(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	host := mcp.NewServer(&mcp.Implementation{Name: "host", Version: "test"}, &mcp.ServerOptions{
		Instructions: mcpserver.Instructions + "\n\nhost_echo echoes text.",
	})
	if err := mcpserver.Register(host, env.opts); err != nil {
		t.Fatal(err)
	}
	mcp.AddTool(host, &mcp.Tool{Name: "host_echo", Description: "Echo text."},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoArgs) (*mcp.CallToolResult, echoResult, error) {
			return nil, echoResult(in), nil
		})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := host.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	cs := connect(t, nil, clientTransport)

	if got := cs.InitializeResult().ServerInfo.Name; got != "host" {
		t.Errorf("server name = %q, want the host's", got)
	}
	if !strings.HasPrefix(cs.InitializeResult().Instructions, mcpserver.Instructions) {
		t.Error("host instructions should carry SDD's instructions text")
	}
	want := append(toolNames(t, connect(t, env.srv)), "host_echo")
	slices.Sort(want)
	if got := toolNames(t, cs); !slices.Equal(got, want) {
		t.Errorf("tools = %v, want SDD's tools plus host_echo %v", got, want)
	}

	openSession(t, cs)
	var echo echoResult
	call(t, cs, "host_echo", map[string]any{"text": "hello"}, &echo)
	if echo.Text != "hello" {
		t.Errorf("host_echo = %q, want hello", echo.Text)
	}
}

func TestRegisterRequiresServer(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	if err := mcpserver.Register(nil, env.opts); err == nil {
		t.Fatal("Register without a server should fail")
	}
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}
