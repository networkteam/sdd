# Engine-native rule integration

## Purpose

This design augments the graph-resident rule-system plan with a procedure-native discovery, application, evidence, and usage flow. It preserves the settled rule representation—required enforcement plus activation.when and/or activation.matches—and replaces the earlier skill-injected index and optional sub-skill delivery path.

## Rule discovery checkpoint

Rule discovery is a mandatory checkpoint declared by relevant procedure steps. Rules do not name procedure canonicals or step IDs.

A checkpoint receives:

- mandatory work-shape topics declared by the step, optionally templated from typed state;
- situation sentences reported by the agent, phrased as the work-side inverse of activation.when;
- proposed, changed, or reviewed artifact paths reported by the agent;
- entry kind and capture operation where the procedure already knows them.

Work contexts reuse hierarchical work-shape topic paths. Rule selectors are component-prefixes such as architecture, implementation, or evaluation/inner; runtime topics can be more specific, such as implementation/engine. Artifact paths are matched by generic file/path globs.

## Discover → Inspect → Apply

### Discover

The engine mechanically computes the complete eligible set using structured selectors. Selector families combine with AND; values within a family are alternatives. A missing selector does not constrain. Missing artifact information is explicit rather than silently treated as an empty set.

The engine serves every eligible rule slimly:

- rule ID;
- enforcement;
- activation.when.

It also names previously applied rules that became ineligible as deactivated. V1 serves the complete slim set rather than an added/removed/unchanged delta, keeping each checkpoint self-contained across resume and context compaction.

The engine retains the runtime match trace—what actual topic/path satisfied each selector—but does not repeat it in the normal serve.

### Inspect

The agent requests candidate IDs. The engine serves each requested rule in full: frontmatter, all activation selectors, and body.

Full content is deduplicated by rule ID plus content hash within a connection. Changed content is re-served. On resume or a new connection, currently applied rule bodies are served once again because prior conversation context may be absent. The match trace is served only for diagnostics, contests, or validation.

### Apply

After inspection, the agent reports applied or contested rules with a specific reason. Mechanically eligible binding rules must be accounted for and cannot be silently omitted. Applied rules remain active across ordinary procedure steps.

## Reconsideration

The engine persists the mechanically eligible set as sorted rule ID plus content-hash fingerprints. Changes to topics, paths, or the rule corpus trigger a cheap recomputation. Discovery is reconsidered only when the effective eligible set changes; raw input changes yielding the same set do not invalidate it.

Situation sentences have a separate semantic-input fingerprint. Changing them requires semantic reconsideration even when the mechanical set is stable.

## Semantic applicability

The normal host-model turn compares the agent's situation sentences with activation.when over the mechanically narrowed slim set. This makes semantic discovery mandatory and evidenced without introducing a separate LLM invocation. Embeddings may later prefilter a large advisory corpus, but are an optimization rather than an enforcement or correctness boundary.

## Evidence and enforcement

The engine does not pass arbitrary work products to pre-flight and the judge must not imply it inspected unseen work. Instead, procedures collect self-describing evidence for each applied rule:

- rule ID;
- disposition;
- why and how the rule was followed;
- relevant artifact paths.

Deterministic gates enforce complete accounting for mechanically eligible binding rules. Missing evidence or an admitted violation can block. An independent judge checks evidence specificity, internal coherence, and graph-anchored claims. Doubts requiring unseen work remain advisory.

## Durable usage

Whenever applied rules shape a capture, the captured entry materializes the evidence in its body. A detailed attachment is only a density escape hatch. The entry adds grounded-in refs to all applied rules, reusing graph heat and decay as rule-usage statistics.

For implementation and evaluation, the closing done signal is the natural evidence carrier. Plans and other entries shaped during capture carry their own applied-rule refs and evidence. Considered, rejected, or contested rules remain in the session record and receive no usage ref.

## Implementation surface

The implementation must add:

- structured work-topic and path selectors;
- rule-discovery checkpoint transitions and schemas;
- eligibility computation and fingerprints;
- slim and full serve behavior with connection-aware dedup;
- applied/contested state and deactivation;
- evidence validation and capture seeding;
- grounded-in refs for applied rules;
- replay, resume, invalidation, and gate tests.

Contract removal and migration remain outside this augmentation, as in the owning plan.
