// Package bundledskills renders the agent-neutral skill templates embedded in
// the sdd binary into per-agent skill bundles.
//
// The source of truth is one neutral template tree under templates/ (files
// named *.md.tmpl). "claude" is no longer a source directory — it is a render
// profile: a model.AgentTarget value, the inject helper's per-agent output, and
// (at install) the frontmatter stamp shape. Load executes the templates for a
// given target; per-agent deviations live in {{ if eq .Agent "..." }}
// conditionals and the inject helper, never as duplicated files. //go:embed
// compiles the templates into the binary; sdd init renders + stamps them into
// the target agent's skill directory.
//
// Editing: edit the templates under templates/<skill>/. After changes, rebuild
// the binary and run ./bin/sdd init to refresh the installed copy under
// .claude/skills/ (or ~/.claude/skills/).
package bundledskills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// templatesFS holds the embedded agent-neutral skill template tree.
//
// Plain `//go:embed templates` recurses the whole tree but excludes files whose
// names start with '.' or '_' (Go's default), keeping OS junk like a
// `.DS_Store` out of the embedded FS. To ship a template's own dotfile, add an
// explicit pattern naming that file.
//
//go:embed templates
var templatesFS embed.FS

const templatesRoot = "templates"

// templateExt is the extension carried by source template files. It is stripped
// from the output path so a file at templates/sdd/SKILL.md.tmpl renders to the
// bundle entry sdd/SKILL.md.
const templateExt = ".tmpl"

// renderData is the value passed to every skill template at execution.
type renderData struct {
	// Agent is the render target's identifier (e.g. "claude"), used by
	// {{ if eq .Agent "claude" }} conditionals to express per-agent prose
	// deviations from one source.
	Agent string
}

// Load renders the embedded skill templates for the given agent target into a
// SkillBundle. Each *.md.tmpl file is parsed and executed with the target's
// render data and helper set; cross-file includes (`{{ template "rel/path" . }}`)
// resolve within the same skill. The returned entries carry no install stamps —
// stamping happens at install time (see model.RenderSkillFile).
func Load(target model.AgentTarget) (*model.SkillBundle, error) {
	switch target {
	case model.AgentClaude:
		// supported
	default:
		return nil, fmt.Errorf("unsupported agent target: %s", target)
	}

	funcs := renderFuncs(target)
	data := renderData{Agent: string(target)}

	skills, err := skillDirs()
	if err != nil {
		return nil, err
	}

	bundle := &model.SkillBundle{Target: target}
	for _, skill := range skills {
		entries, err := renderSkill(skill, funcs, data)
		if err != nil {
			return nil, err
		}
		bundle.Entries = append(bundle.Entries, entries...)
	}
	return bundle, nil
}

// renderFuncs builds the template FuncMap bound to a specific target. inject
// renders a CLI command as Claude's dynamic-injection token (`!`+"`cmd`"+`) that
// Claude Code expands at skill-load, or — for agents without that mechanism — as
// an explicit instruction to run the command. Binding the target via closure
// lets templates call `{{ inject "sdd info" }}` without threading the agent
// through every call site.
func renderFuncs(target model.AgentTarget) template.FuncMap {
	return template.FuncMap{
		"inject": func(cmd string) string {
			if target == model.AgentClaude {
				return "!`" + cmd + "`"
			}
			return "Run `" + cmd + "` and use its output as context."
		},
	}
}

// skillDirs returns the immediate subdirectories of templates/, one per skill.
func skillDirs() ([]string, error) {
	entries, err := templatesFS.ReadDir(templatesRoot)
	if err != nil {
		return nil, fmt.Errorf("reading templates root: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// renderSkill parses every template file under one skill into a shared template
// set (so `{{ template }}` includes resolve across files), then executes each
// file with data. Templates are named by their skill-relative output path
// (`.tmpl` stripped), e.g. "SKILL.md", "references/ref-kinds.md". Parsing all
// files before executing any lets a file include another regardless of order.
func renderSkill(skill string, funcs template.FuncMap, data renderData) ([]model.SkillBundleEntry, error) {
	skillRoot := path.Join(templatesRoot, skill)
	set := template.New(skill).Funcs(funcs)

	var outRels []string
	err := fs.WalkDir(templatesFS, skillRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := relUnder(skillRoot, p)
		if err != nil {
			return err
		}
		outRel := strings.TrimSuffix(rel, templateExt)

		content, err := templatesFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", p, err)
		}
		if _, err := set.New(outRel).Parse(string(content)); err != nil {
			return fmt.Errorf("parsing template %s: %w", p, err)
		}
		outRels = append(outRels, outRel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	entries := make([]model.SkillBundleEntry, 0, len(outRels))
	for _, outRel := range outRels {
		var buf bytes.Buffer
		if err := set.ExecuteTemplate(&buf, outRel, data); err != nil {
			return nil, fmt.Errorf("executing template %s/%s: %w", skill, outRel, err)
		}
		entries = append(entries, model.SkillBundleEntry{
			Skill:   skill,
			RelPath: outRel,
			Content: buf.Bytes(),
		})
	}
	return entries, nil
}

// ReadReference returns the body (frontmatter stripped) of a reference template
// in the embedded bundle. Pre-flight uses this to inject the canonical ref-kind
// vocabulary into its prompt from the same source the skill ships. The reference
// is read raw rather than executed — the vocabulary fragment is agent-neutral
// and carries no template directives, so its source bytes equal its rendered
// output.
func ReadReference(skill, relPath string) ([]byte, error) {
	full := path.Join(templatesRoot, skill, relPath+templateExt)
	data, err := templatesFS.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("reading bundled reference %s: %w", full, err)
	}
	return model.SkillFileBody(data), nil
}

// relUnder returns full expressed relative to root, with leading separators
// trimmed and the path cleaned.
func relUnder(root, full string) (string, error) {
	rel := strings.TrimPrefix(full, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", fmt.Errorf("unexpected empty relative path for %q", full)
	}
	return path.Clean(rel), nil
}
