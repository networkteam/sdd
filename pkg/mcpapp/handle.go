package mcpapp

import (
	"strings"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// handleSeparator joins a project ID and a session ID into the handle the
// wrapper serves. The application's session ID is project-local; the wrapper
// serves one endpoint for every project a principal can reach, so the handle
// must carry the project for the doors that load a session from its store
// (d-tac-1z6). Session IDs start with "s_" and never contain ":", so splitting
// at the last ":s_" is unambiguous whatever the project ID contains.
const handleSeparator = ":"

func composeHandle(project sdd.ProjectID, id sdd.SessionID) string {
	if project == "" {
		return string(id)
	}
	return string(project) + handleSeparator + string(id)
}

// splitHandle returns the project and session ID a handle names. A bare
// session ID yields an empty project: the composition's pinned project or the
// application's sole-project inference supplies it.
func splitHandle(handle string) (sdd.ProjectID, sdd.SessionID) {
	handle = strings.TrimSpace(handle)
	if at := strings.LastIndex(handle, handleSeparator+"s_"); at >= 0 {
		return sdd.ProjectID(handle[:at]), sdd.SessionID(handle[at+len(handleSeparator):])
	}
	return "", sdd.SessionID(handle)
}
