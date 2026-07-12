package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// requestIdentity is the protocol-neutral authentication material carried by
// the current MCP request. It deliberately stays private until the public SDD
// application contract lands; the transport spike exercises this exact bridge
// so later extraction does not fall back to initialization-time identity.
type requestIdentity struct {
	Subject    string
	Scopes     []string
	Attributes map[string]any
}

func identityFromRequest(req *mcp.CallToolRequest) requestIdentity {
	var identity requestIdentity
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return identity
	}
	info := req.Extra.TokenInfo
	identity.Subject = info.UserID
	identity.Scopes = append([]string(nil), info.Scopes...)
	if len(info.Extra) > 0 {
		identity.Attributes = make(map[string]any, len(info.Extra))
		for key, value := range info.Extra {
			identity.Attributes[key] = value
		}
	}
	return identity
}
