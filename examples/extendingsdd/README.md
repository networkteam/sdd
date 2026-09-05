# Extending SDD from another Go module

This nested module compiles as `example.com/extendingsdd`, so it proves that an application can embed SDD without importing repository-internal packages.

The composition in `main.go` owns:

- authentication middleware and token validation;
- principal, project, access, and dependency resolution, and the session-continuation policy (`AuthorizeSession`; SDD ships `application.OwnerOnly`);
- graph, LLM, embedding/index, and finalizer adapters per project, and one session store and one staged-blob store for the whole composition;
- the HTTP listener and deployment policy.

SDD owns graph semantics, workflow procedures, read rendering, pre-flight and summary behavior, durable transition recovery, and write gates. The composition imports the canonical `application` contracts and optional `local` adapters, constructs an `application.ProjectRuntime` per project, resolves them through `application.AccessResolver`, creates `application.Application` with the composition-wide session and staged-blob stores, and mounts `mcpapp.Server.Handler()` behind the MCP SDK's bearer middleware. A session handle is the session ID alone; the application reads a session's home project from its own record, so the server pins no project and serves every project the principal can reach.

For local development the module uses `replace github.com/networkteam/sdd => ../..`. A release consumer removes that replacement and pins a published SDD version.

Search freshness and background index maintenance are host choices. The example
selects synchronous search through `mcpapp.Options.SearchSyncMode`. See the
[application package documentation](../../pkg/application/doc.go) for the API and
synchronization modes.

Run its independent compile and port-surface checks from this directory:

```bash
go test ./...
```
