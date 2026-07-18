package engine

import (
	"fmt"
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// FuncClass is one of the registry's three function classes.
type FuncClass string

const (
	ClassPredicate FuncClass = "predicate"
	ClassQuery     FuncClass = "query"
	ClassCommand   FuncClass = "command"
)

// FuncDoc is a registry function's documented contract: what it reads from
// and writes into the store. YAML never maps fields — it only names
// functions; these contracts are what spec authors consult (registryList,
// the MCP registry tool) and what load-time collision checks enforce.
type FuncDoc struct {
	Name   string
	Class  FuncClass
	Doc    string
	Reads  []string
	Writes []string
}

// Context is what registry functions see: the running instance's store and
// the graph, read-only by convention for predicates and queries. Commands
// additionally reach their side-effect dependencies through the closures
// they were registered with — the registry itself stays dependency-free.
type Context struct {
	Store *Store
	Graph *model.Graph
	// Step is the step the function runs at — commands like confirmPlayback
	// record it (the reopen target when a confirmation goes stale).
	Step string
	// Reads is the session's folded read set — the deepest depth each entry
	// was served at (refsInspected evaluates it). Nil means nothing was
	// served; reading a nil map is safe.
	Reads map[string]ReadDepth
}

// PredicateFunc is a pure check over the store (and graph): no side effects,
// deterministic for a given state.
type PredicateFunc func(ctx *Context) (bool, error)

// QueryFunc serves data for dynamic injection: pure read, result enters the
// template context (not the store).
type QueryFunc func(ctx *Context, args map[string]any) (any, error)

// CommandFunc executes a side effect (gate ops and chooser calls only). Its
// store writes go through Store.WriteEngine per the declared contract.
type CommandFunc func(ctx *Context) error

// Predicate is a registered predicate with its contract and failure text —
// the message served as instruction when the predicate stalls a gate.
type Predicate struct {
	Doc         FuncDoc
	Fn          PredicateFunc
	FailMessage string
	// FailDetail, when set, renders the failure message against the live
	// context — for gates whose rejection must name the offending values
	// (refsInspected naming the un-inspected IDs). Empty results fall back
	// to FailMessage.
	FailDetail func(ctx *Context) string
}

// Query is a registered query.
type Query struct {
	Doc FuncDoc
	Fn  QueryFunc
	// ServeSafe marks the query as a pure read with no side effects — it writes
	// nothing to the session log or graph. Only serve-safe queries may be
	// declared as shell framing lanes: framing renders on every serve, and a
	// serve must not write (I7). A query that logs its reads (LogRead) or
	// mutates is not serve-safe and is rejected as a framing lane at spec load.
	ServeSafe bool
}

// Command is a registered command.
type Command struct {
	Doc FuncDoc
	Fn  CommandFunc
	// MutatesGraph declares that running this command changes the on-disk graph
	// (a new entry, a rewritten summary). The engine invalidates the graph
	// provider after such a command so later reads in the same advance reload
	// and see the write — post-write freshness driven by the command's own
	// declaration, replacing the shell's hand-maintained refresh calls.
	MutatesGraph bool
}

// Registry is the closed set of Go functions a procedure spec may name. All
// semantics live here, composed by name in YAML — the spec language itself
// carries no logic. Enumerable so spec authors and lint can check every
// referenced name.
type Registry struct {
	predicates map[string]*Predicate
	queries    map[string]*Query
	commands   map[string]*Command
}

// NewRegistry returns a registry pre-populated with the built-in predicates
// (largely the mechanical pre-flight checks re-exposed — single path, no
// duplicated validation). Queries and commands with side-effect dependencies
// are registered by the shell that owns those dependencies.
func NewRegistry() *Registry {
	r := &Registry{
		predicates: map[string]*Predicate{},
		queries:    map[string]*Query{},
		commands:   map[string]*Command{},
	}
	registerBuiltinPredicates(r)
	registerBuiltinCommands(r)
	registerBuiltinQueries(r)
	return r
}

// RegisterPredicate adds a predicate. Names are unique across all classes.
func (r *Registry) RegisterPredicate(p Predicate) error {
	if err := r.checkName(p.Doc.Name); err != nil {
		return err
	}
	p.Doc.Class = ClassPredicate
	r.predicates[p.Doc.Name] = &p
	return nil
}

// RegisterQuery adds a query. Names are unique across all classes.
func (r *Registry) RegisterQuery(q Query) error {
	if err := r.checkName(q.Doc.Name); err != nil {
		return err
	}
	q.Doc.Class = ClassQuery
	r.queries[q.Doc.Name] = &q
	return nil
}

// RegisterCommand adds a command. Names are unique across all classes.
func (r *Registry) RegisterCommand(c Command) error {
	if err := r.checkName(c.Doc.Name); err != nil {
		return err
	}
	c.Doc.Class = ClassCommand
	r.commands[c.Doc.Name] = &c
	return nil
}

func (r *Registry) checkName(name string) error {
	if !isValidFuncName(name) {
		return fmt.Errorf("invalid registry function name %q", name)
	}
	if _, ok := r.predicates[name]; ok {
		return fmt.Errorf("registry name %q already taken by a predicate", name)
	}
	if _, ok := r.queries[name]; ok {
		return fmt.Errorf("registry name %q already taken by a query", name)
	}
	if _, ok := r.commands[name]; ok {
		return fmt.Errorf("registry name %q already taken by a command", name)
	}
	return nil
}

// Predicate looks up a predicate by name.
func (r *Registry) Predicate(name string) (*Predicate, bool) {
	p, ok := r.predicates[name]
	return p, ok
}

// Query looks up a query by name.
func (r *Registry) Query(name string) (*Query, bool) {
	q, ok := r.queries[name]
	return q, ok
}

// Command looks up a command by name.
func (r *Registry) Command(name string) (*Command, bool) {
	c, ok := r.commands[name]
	return c, ok
}

// Docs enumerates registered function contracts, optionally filtered by
// class, sorted by name — the registryList query's data.
func (r *Registry) Docs(class FuncClass) []FuncDoc {
	var docs []FuncDoc
	if class == "" || class == ClassPredicate {
		for _, p := range r.predicates {
			docs = append(docs, p.Doc)
		}
	}
	if class == "" || class == ClassQuery {
		for _, q := range r.queries {
			docs = append(docs, q.Doc)
		}
	}
	if class == "" || class == ClassCommand {
		for _, c := range r.commands {
			docs = append(docs, c.Doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	return docs
}

// registerBuiltinQueries registers the dependency-free built-ins. Queries
// that need finders or session framing (viewLayout, entryChains,
// sessionInfo, generatedSummary) are registered by the shell that wires
// those dependencies.
func registerBuiltinQueries(r *Registry) {
	mustRegisterQuery(r, Query{
		Doc: FuncDoc{
			Name: "registryList",
			Doc:  "Function contracts per class — what spec authors consult. Optional arg class: predicate|query|command.",
		},
		ServeSafe: true,
		Fn: func(_ *Context, args map[string]any) (any, error) {
			class, _ := args["class"].(string)
			switch FuncClass(class) {
			case "", ClassPredicate, ClassQuery, ClassCommand:
				return r.Docs(FuncClass(class)), nil
			default:
				return nil, fmt.Errorf("unknown registry class %q", class)
			}
		},
	})
}

func mustRegisterPredicate(r *Registry, p Predicate) {
	if err := r.RegisterPredicate(p); err != nil {
		panic(err)
	}
}

func mustRegisterQuery(r *Registry, q Query) {
	if err := r.RegisterQuery(q); err != nil {
		panic(err)
	}
}

func mustRegisterCommand(r *Registry, c Command) {
	if err := r.RegisterCommand(c); err != nil {
		panic(err)
	}
}
