package mcpapp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd"
)

// requestIdentity is the MCP-shaped authentication material carried by the
// current request. Keeping extraction private prevents protocol SDK types from
// leaking into the public application contract.
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

func publicIdentityFromRequest(req *mcp.CallToolRequest) sdd.RequestIdentity {
	identity := identityFromRequest(req)
	return sdd.RequestIdentity{Subject: identity.Subject, Scopes: identity.Scopes, Attributes: identity.Attributes}
}

func (s *Server) requestIdentity(req *mcp.CallToolRequest) sdd.RequestIdentity {
	identity := publicIdentityFromRequest(req)
	if identity.Subject == "" && s.localIdentity.Subject != "" {
		return s.localIdentity
	}
	return identity
}
