# Extending SDD from another Go module

This nested module compiles as `example.com/extendingsdd`, so it proves that an application can embed SDD without importing repository-internal packages.

The composition in `main.go` owns:

- authentication middleware and token validation;
- principal, project, access, and dependency resolution;
- graph, session, staged-blob, LLM, embedding/index, and finalizer adapters;
- the HTTP listener and deployment policy.

SDD owns graph semantics, workflow procedures, read rendering, pre-flight and summary behavior, durable transition recovery, and write gates. The composition imports the canonical `application` contracts and optional `local` adapters, constructs an `application.ProjectRuntime`, resolves it through `application.AccessResolver`, creates `application.Application`, and mounts `mcpapp.Server.Handler()` behind the MCP SDK's bearer middleware.

For local development the module uses `replace github.com/networkteam/sdd => ../..`. A release consumer removes that replacement and pins a published SDD version.

Run its independent compile and port-surface checks from this directory:

```bash
go test ./...
```
