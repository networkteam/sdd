# Wild sweep of stored summaries against the summary contract (d-cpt-75a)

Run 2026-09-01 on this repo's graph. Instrument: TestSummarizeWildSweep
(internal/llmops/summarize_wild_eval_test.go) — deterministic stratified sample
(per kind, evenly spaced over the chronological list) of 50 stored summaries;
each judged against the contract's axes by the fixed judge
(ollama glm-5.3-flash:cloud, think=high, subscription-covered), one retry on
unparseable output; mechanical checks (self-ID, labels, quotes, markdown,
word bounds) run beside the judge. Re-run: same command, selection is
deterministic, so post-regeneration runs compare like for like.

## Aggregate

50 judged, 29 judge-fail (58%), 1 mechanical-fail, 0 infrastructure failures.

Axis failures (a failed summary can fail several):

| Axis | Failures |
|---|---|
| lede_standalone (first sentence stuffed, buried, or metadata-led) | 23 |
| grounded (asserts what the body does not state) | 13 |
| concrete_wording (jargon, inflated or vague phrasing) | 10 |
| fit_covered (relationship coverage) | 3 |
| terse | 2 |

By entry month (fail/pass): Apr 13/8 · May 4/2 · Jun 3/4 · Jul 7/6 · Aug 2/1 —
no age trend; every template era misses the new bar at a similar rate.

Recurring patterns worth naming beyond the axes: positioning language
transferred from neighbors' summaries into new generations (drift feeding
drift through the prompt's related-entries block); tense inflation (a plan
summarized as "Implements..."); and detailed figures — CLI commands, token
counts, durations — carried into summaries whose bodies own that detail.

## All 29 failures, verbatim
### 20260406-115516-s-stg-beh [gap/strategic]

**Judge:** grounded: second sentence asserts a prescription the source does not state — that this 'requires re-evaluation of workflow structures and artifact requirements'; the entry only observes that the coding-bottleneck constraint no longer applies with agents, not that any action is required

**Stored summary:** Development practices rooted in Scrum and Agile frameworks now generate unnecessary overhead, as these methods assumed coding was the primary bottleneck—a constraint that no longer holds with agent-based development. This signals a strategic gap between current process design and the capabilities enabled by AI agents, requiring re-evaluation of workflow structures and artifact requirements.

### 20260406-115540-d-stg-0gh [directive/strategic]

**Judge:** Grounded: 'over waterfall planning' invents a 'waterfall' label the source never uses — the refs name Scrum/Agile overhead and 'sequential planning', not waterfall; Grounded: 'rigidity concerns surfaced in the Scrum-Agile gap' attributes rigidity to 20260406-115516-s-stg-beh, whose stated concerns are unnecessary overhead and a stale coding-bottleneck assumption, not rigidity

**Stored summary:** Commits to designing a first-principles process framework for agentic product development across the full lifecycle, organized around the Signal → Dialogue → Decision → Action loop operating at multiple abstraction layers. This decision directly closes the overhead and rigidity concerns surfaced in the Scrum-Agile gap (20260406-115516-s-stg-beh), the overplanning bias (20260406-115521-s-stg-9pd), the sequential execution bottleneck (20260406-115528-s-stg-8d3), and the human oversight gap (20260406-115534-s-stg-cj7), establishing a unified framework that prioritizes continuous measurement and concurrent feedback over waterfall planning.

### 20260406-115547-d-cpt-o8t [contract/conceptual]

**Judge:** Grounded: the second sentence asserts the entry 'enables the Signal-Dialogue-Decision-Action loop' — the loop appears only in the referenced strategic directive's summary, not in the entry itself; the entry states nothing about enabling the loop.; Grounded: 'Operationalizes the strategic framework directive' is invented positioning — the entry is a conceptual-layer contract that describes the immutable graph structure; 'operationalizes' mischaracterizes it, and the 'technical contract' framing is not in the source.; Lede: the first sentence stacks four facts in stacked clauses (immutable Git graph, three types and five layers, supersede-not-modify, state reconstructed by traversal) instead of compressing to the single most important meaning.; Concrete wording: 'Operationalizes' and 'single-source-of-truth' are inflated/jargon phrasing; the source says 'each fact lives in exactly one place' — plainer wording should carry the claim.

**Stored summary:** Commits to an immutable decision graph structure using Git with three entry types across five abstraction layers, where documents are never modified but superseded, and current state is always reconstructed from the graph traversal rather than maintained separately. Operationalizes the strategic framework directive (20260406-115540-d-stg-0gh) by establishing the technical contract that enables the Signal-Dialogue-Decision-Action loop and ensures single-source-of-truth fact storage across all layers.

### 20260406-115559-s-cpt-f8v [done/conceptual]

**Judge:** Grounded: the second sentence invents a relationship the source does not state — the entry only carries plain refs to 20260406-115540-d-stg-0gh and 20260406-115547-d-cpt-o8t, but the summary claims the work 'operationalizes' both decisions and 'supports the immutable decision graph structure', which is graph-positioning commentary transferred from the neighbors' own summaries, not the source material.; Fit_covered: the fit it describes is fabricated — the claimed operationalizing/support relationship to the two referenced decisions is asserted, not grounded, so the entry's actual fit (whatever the refs mean) is not honestly covered.; Lede_standalone: the first sentence is a long stacked-clause list stuffing all four deliverables (framework definition, story, signals, sdd-new.sh script) into one sentence instead of leading with the single most important meaning in the shortest carrying sentence; it also adds 'initial', which the source does not state.

**Stored summary:** Completed initial framework documentation and tooling by writing the core Signal-Dialogue-Decision framework definition, a practical coffee roastery team story, and collected design signals and open questions, plus built the sdd-new.sh script for graph entry creation. This work operationalizes the strategic framework directive (20260406-115540-d-stg-0gh) and the conceptual contract decision (20260406-115547-d-cpt-o8t) by providing concrete definitions and automation to support the immutable decision graph structure.

### 20260407-144641-s-prc-ggp [question/process]

**Judge:** Lede stacks two facts into one sentence: it both raises the commit-granularity question and proposes commit-hash linking; the single most important meaning is the question itself and should lead alone; Second sentence is largely redundant with the first — 'decision-to-code traceability' and the Git-history link are already stated, so the clause adds no new information

**Stored summary:** Raises the question of Git commit granularity for graph entries and proposes linking implementation actions to commit hashes for traceability between decisions and code changes (d-cpt-o8t). This explores how to operationalize the immutable graph structure commitment by connecting graph entries directly to Git history, establishing decision-to-code traceability within the technical contract framework.

### 20260408-100259-s-ops-h63 [gap/operational]

**Judge:** Grounding: the summary attaches the ref ID 20260408-100254-s-tac-a0c to the signal split ('split a bundled signal into two focused ones (20260408-100254-s-tac-a0c)'), but the source never states that the split produced or involves that entry; a0c is only listed as the entry's ref.; Fit: after the lede, the summary never describes how the entry relates to its directly referenced entry a0c (the done signal that delivered the explore capability this first use exercised); the ID appears only in a misattributed spot.; Lede: the first sentence stacks two separate facts ('successfully surfaced hidden semantic connections across four entries spanning multiple layers' and 'split a bundled signal into two focused ones') into one compound sentence instead of compressing to the single most important meaning.; Concrete wording: 'successfully' is an inflated evaluation not present in the source; 'requiring deduplication logic to clean redundant strategic signals appearing multiple times' pads a simple 'should deduplicate across sections' with redundant phrasing ('duplicated' already implies redundancy).

**Stored summary:** Explore mode's first use on s-prc-ggp successfully surfaced hidden semantic connections across four entries spanning multiple layers and split a bundled signal into two focused ones (20260408-100254-s-tac-a0c). The sub-skill output duplicated upstream chain context across sections, requiring deduplication logic to clean redundant strategic signals appearing multiple times.

### 20260408-173303-s-cpt-asd [done/conceptual]

**Judge:** The source says the convention 'hasn't been a problem in practice'; the summary upgrades this to 'proven effective in practice,' asserting a stronger claim than the source supports.

**Stored summary:** Entry size convention is already addressed by existing guidance in meta-process.md and /sdd skill instructions, establishing the principle of one idea per entry with externalized detail. This resolves the conceptual gap on entry granularity (20260406-232047-s-cpt-qcq), confirming that no formal decision is needed as the convention has emerged naturally through skill design and proven effective in practice.

### 20260410-214052-d-tac-l5b [plan/tactical]

**Judge:** Lede asserts 'Implements' but the source entry is a plan (Kind: plan, 'Plan for --attach filename mapping'); the summary states work as done when the entry only commits to it.; Lede stacks multiple facts into stacked 'by' clauses ('by converting..., parsing..., and supporting...') instead of a single compressed statement of the plan's substance.; 'operationalizing its CLI workflows' is inflated jargon; the plan's actual relationship is implementing the --attach filename mapping under the attachment mechanism the directive established.; 'colon-prefixed targets' is loose wording for the source's requirement that the stdin alias '-' requires a :target suffix.

**Stored summary:** Implements filename mapping for the --attach CLI flag by converting it to a repeatable StringSliceFlag, parsing source:target syntax with basename fallback, and supporting stdin aliases with colon-prefixed targets. Closes the meaningless attachment naming gap (20260410-151158-s-tac-lr5) and builds directly on the attachment infrastructure directive (20260409-113337-d-tac-04t) by operationalizing its CLI workflows with tests, documentation updates, and binary rebuild.

### 20260413-161531-d-cpt-uu1 [directive/conceptual]

**Judge:** grounded: claims the Claude Code plugin deferral is 'to post-MVP', but the source only defers the self-update subcommand post-MVP; the plugin marketplace is 'deferred as an optional secondary channel' with no stated timing; lede_standalone: the first sentence stuffs five facts into one clause chain (dual channels, skills extraction, bundled migrations); the core commitment is the dual-channel strategy, and skills/migrations belong in a following sentence; concrete_wording: 'operationalizing findings ... into a concrete agent-neutral delivery model' is jargon-laden and vague — the source says the research findings 'drove' the channel choices, which could be stated in plain words

**Stored summary:** Commits to a dual-channel distribution strategy for SDD: GoReleaser-driven GitHub Releases feeding a custom Homebrew tap for brew users, plus a binstaller-generated install.sh for curl-install users, with embedded skills extracted on installation and schema migrations bundled in the binary. Closes the distribution tooling gap (s-cpt-4gj) by operationalizing findings from the seven-angle research (s-cpt-xbv) into a concrete agent-neutral delivery model that honors the platform-neutrality requirement (d-stg-574) and sequences after the SDD fork (d-cpt-dlw), while deferring self-update subcommands and Claude Code plugin discovery to post-MVP.

### 20260416-203327-s-prc-v69 [gap/process]

**Judge:** Typo in second sentence: 'This gaps connects' should read 'This gap connects' — sloppy wording that would propagate into future sessions

**Stored summary:** The sdd status and sdd list commands omit kind and participants fields per line, requiring consumers like /sdd-catchup and agent queries to fetch full entries via sdd show, creating wasted round-trips and LLM budget loss. This gaps connects to participant naming inconsistency (s-prc-omw) which shares the remediation path, and aligns with the broader push to surface deterministic facts outside the LLM layer (s-prc-hpa). The output format should prioritize agent consumption over human readability by ensuring uniform presence of structurally relevant fields.

### 20260422-122136-d-stg-beb [aspiration/strategic]

**Judge:** Grounded: the second sentence claims the aspiration 'operationalizes the type system contract (d-cpt-ydf) by grounding reasoning-first' — the source only lists d-cpt-ydf as a ref and never states any operationalizing or grounding relationship to that contract; this invented fit commentary propagates a false relationship into the graph.

**Stored summary:** This strategic aspiration commits SDD to dialogue-first reasoning: decisions emerge from multi-party engagement, not retrieval or solo generation alone, with all tooling serving rather than replacing that dialogue. It closes the positioning gap (s-stg-gtu) by articulating that retrieval-adjacent capabilities enable better dialogue rather than operating as reasoning themselves, and operationalizes the type system contract (d-cpt-ydf) by grounding "reasoning-first" as a consequence of dialogue shaping decisions.

### 20260422-235706-d-cpt-x38 [plan/conceptual]

**Judge:** lede_standalone: the first sentence stacks several facts into one long multi-clause construction (commitment to multilingual support, config-file location, capture-authoring rule, English CLI tokens, and on-demand vocabulary rendering), instead of compressing to the single most important meaning — e.g. 'This plan commits SDD to multilingual support via a per-graph configured language.'

**Stored summary:** This plan commits SDD to multilingual support via per-graph language configuration in `.sdd/config.yaml`, with captured entries authored in the configured language while CLI tokens remain English canonical identifiers and the skill renders translated vocabulary on demand (d-cpt-ydf). It resolves the multilingual question (s-cpt-y33) by specifying capture-time canonicalization, language-drift detection in pre-flight, and surfacing the configured language in `sdd status` headers, directly supporting the non-technical access aspiration (d-stg-x0l) while maintaining agent-neutrality foundations (d-stg-574).

### 20260424-102831-d-prc-hfs [role/process]

**Judge:** Lede stacks multiple facts into one sentence ('designated designer... holding strategic ownership while delegating... with authority over all strategic direction, implementation, and entry review decisions') instead of compressing to the single core claim: Christopher holds strategic and conceptual ownership of the SDD framework.; Not grounded: 'delegating tactical and operational work to Claude' misstates the source, which says tactical and operational work happens collaboratively with Claude — delegation of authority is not stated.; Not grounded: 'authority over all ... implementation' inflates the source, which says Christopher writes the Go implementation himself rather than holding authority over implementation decisions.; Concrete wording: 'role allocation' and 'clarifies the division of decision-making authority' are abstract, bureaucratic phrasings rather than concrete statements of the entry's content.

**Stored summary:** Christopher is designated designer and principal developer of the SDD framework, holding strategic and conceptual ownership while delegating tactical and operational work to Claude, with authority over all strategic direction, implementation, and entry review decisions. This role allocation references the actor profile confirmation (20260424-102742-s-prc-hav) and clarifies the division of decision-making authority within the SDD development process.

### 20260505-111057-s-tac-n8u [fact/tactical]

**Judge:** Lede is a verbless fragment ('Token baseline measurement from a Claude Code session...') that labels the entry instead of stating what it found; the substance (skill metadata ~1.2k tokens, 7% of 1m context) is deferred to later sentences.; Summary runs to seven sentence-like units (including the 'Memory files: 3.3k' / 'System prompt: 8.6k' / 'System tools: 11.3k' fragments), far exceeding the three-sentence limit; the token breakdown should be compressed into one or two dense sentences.; Note: no fit coverage is required here since the source entry states no relationship between the measurement and the referenced process directive; omitting it is correct.

**Stored summary:** Token baseline measurement from a Claude Code session (Opus 4.7, 1m context, 2026-05-05) immediately after `/sdd` invocation. SDD-family skill metadata costs ~1.2k tokens combined at the trigger surface (5 skills × ~50 tokens each, name + description only). After /sdd loads, total Messages reach 51.3k tokens, including SKILL.md, the three references it pulls (framework-concepts, meta-process, cli-reference), initial sdd status output, and the agent's first reply. Memory files: 3.3k. System prompt: 8.6k. System tools: 11.3k. Total session usage: 7% of 1m context. The trigger surface is small; the active-skill load is moderate at 1m context and larger proportionally at 200k.

### 20260506-190107-d-tac-0qn [focus/tactical]

**Judge:** Lede stuffs a second fact into a trailing clause: 'with clear involvement patterns across three workstreams' duplicates what sentence 2 details and dilutes the core commitment; the first sentence should end at the three-branch parallel push.; Vague, inflated wording: 'clear involvement patterns' asserts nothing concrete (clarity of a pattern is not stated by the source as such) and propagates fuzz; the sentence already names the concrete involvement states, so the clause is filler.; Minor wording drift: 'validating all three involvement-resolution states' vs the source's 'exercising all three involvement-resolution states' — the summary inflates a smoke test into validation.

**Stored summary:** Commits to a two-week push completing the type-system 7+7 implementation in parallel with the SDD view plan and playbook rewrite, with clear involvement patterns across three workstreams. Christopher and Claude actively drive the type-system work (d-tac-gvn), the SDD view remains pull-available pending the topic-filter primitive (d-tac-uww), and Claude solo handles the playbook rewrite gated on both upstream plans (d-tac-1du). This focus serves as a smoke test validating all three involvement-resolution states.

### 20260520-132400-d-tac-nbp [directive/tactical]

**Judge:** lede_standalone: the first sentence stacks the core commitment together with the Plan-1 dependency timing and the closure of two gap halves (s-cpt-sy4 and s-cpt-ghy) in one multi-clause sentence; the lede should be just the commitment (e.g. 'This directive commits to designing and running a retroactive topic-grooming flow for the existing graph.'), with the dependency and gap closures moved to the fit sentences.

**Stored summary:** This directive commits to designing and running a retroactive topic-grooming flow for the existing graph after Plan-1 (d-tac-6tz) ships its capture-time procedure and view infrastructure, completing the topic-backfill half of the catch-up infrastructure need (s-cpt-sy4) and the grooming half of the topic infrastructure gap (s-cpt-ghy). The grooming surface will survey untagged entries using Plan-1's filter and aggregation tools, identify clusters, propose labels via capture-time procedure, and backfill active entries with annotation entries. Mechanism details—sub-skill vs playbook, cluster-identification approach, and backfill threshold—are deferred to dialogue once Plan-1 ships and the real graph behavior is observable, grounding in the topic primitive contract (d-cpt-ni0).

### 20260529-210808-s-prc-web [gap/process]

**Judge:** Lede stacks multiple facts into one long multi-clause sentence: it packs the selection-default finding, the body-assertion mismatch, the medium-band misfire, and the read-and-respond cost all before stopping. The single most important meaning (ref-kind selection at capture undersells what bodies assert, creating noise) should stand alone in a short first sentence.; Minor groundedness stretch: 's-tac-uer, which owns the durable fix' — the source says s-tac-uer's domain is the single canonical home that the durable selection-rule tightening wants, not that the s-tac-uer question itself owns the fix; this risks a future session treating the open question as the fix owner.

**Stored summary:** Capture-time ref-kind selection defaults to semantically thin `related` labels while bodies assert sharper relationships, and pre-flight's medium-band validation fires on defensible choices rather than genuine errors, producing non-blocking noise that costs read-and-respond cycles. Analysis of 12 `related` refs shows ~5 are correct siblings and ~7 defensible but underspecified, with zero body contradictions, indicating poor signal-to-noise in the medium band (s-tac-aj3). This selection-and-calibration gap interacts with the shared-reference canonical-grounding question (s-tac-uer), which owns the durable fix; together they require tightening the ref-kind choice rule against the authoring-order inversion that undermines pre-flight's three-tier severity model.

### 20260608-004727-s-prc-4kh [gap/process]

**Judge:** The first sentence stuffs three stacked facts (documentation routing, validator flagging, and the five-of-nine closure statistic) into one long clause-chained sentence instead of leading with the single compressed core observation — that insight-closure semantics are inconsistent across documentation, validator, and practice

**Stored summary:** This signal observes a three-way inconsistency in how insight-kind signals are closed: framework documentation routes them through a directive, the validator flags done-signal closures as unusual, yet five of nine historically closed insights were in fact closed by done signals rather than decisions. The gap is grounded in the type-system contract that establishes insight as a distinct observational signal kind (d-cpt-ydf), whose formal vocabulary and validation rules create the very rules now in conflict. Resolution requires either sanctioning done-closes-insight as a first-class closure path or correcting the divergent practice.

### 20260610-225912-s-tac-n9a [done/tactical]

**Judge:** Lede sentence stacks many facts (commit, state machine, four tools, grounding gate, transports, test suite, live verification) into a long multi-clause sentence instead of the single most important meaning — that slice 1 of the workflow MCP server shipped, closing the build activity.; 'bearer-token-secured transports' overstates the source: only the HTTP transport carries the mandatory bearer token; stdio is not token-guarded.; 'full integration test suite' is inflated phrasing — the source enumerates specific covered cases, not a 'full' suite.

**Stored summary:** This signal records that slice 1 of the workflow MCP server shipped in commit f92fed5, delivering `sdd serve` with a per-session state machine, four dialogue-loop tools, structurally enforced grounding gate, bearer-token-secured transports, and a full integration test suite — all verified live against the repo's graph. It closes the build activity (20260610-224323-d-tac-et4) and unblocks the two-client evaluation runs that the workflow-MCP experiment directive was waiting on (20260609-234656-d-cpt-afn).

### 20260628-130400-d-cpt-uh0 [directive/conceptual]

**Judge:** fit_covered: the summary omits the entry's positioning among its referenced entries — it never mentions that this directive re-states the superseded identical directive (d-cpt-ba7) under the one-time intent-backfill stamping (d-tac-9lv), nor that the superseded entry carries the full rationale and refs; the second sentence only expands the rule's content rather than describing how this entry fits among them

**Stored summary:** A universal capture-time invariant: every reference must resolve or capture is blocked with a high-severity precondition finding, applied equally to local and cross-repo refs. It follows from immutability — backward-pointing refs target entries that already exist, so a non-resolving ref is a broken edge — while the forward-class kinds (surfaces, required-by) are exempt because they point at an entry captured first.

### 20260702-204302-s-tac-4yf [done/tactical]

**Judge:** lede_standalone: the first sentence opens with bookkeeping ('This signal records...') and stacks multiple facts into layered clauses — completion, the parity-inventory outcome, the spec-session outcome, plus a five-item enumeration (move spec structure, JSONL event logs, MCP surface, Go registry, dynamic injection). The core meaning 'steps 1 and 2 of the workflow-engine planning activity (20260702-180130-d-tac-tdh) are complete' should stand alone as a short compressed lede, with the outcomes moved to the fit sentences

**Stored summary:** This signal records completion of steps 1 and 2 of the workflow-engine planning activity (20260702-180130-d-tac-tdh): the parity inventory is settled as the plan's scope contract, and the spec session resolved its concrete unknowns — covering move spec structure, session persistence as append-only JSONL event logs, MCP surface design, a closed Go logic registry, and dynamic injection. The session itself dogfooded the structured interview production mode (20260702-174013-s-cpt-qs2), the rule-system sequencing question (20260622-084244-d-tac-eho) is settled as spine-first with rule kind as fast-follow in one type-system revision, session-scoped attachment staging absorbs the orphaned upload-token and read-side surfaces (20260527-212055-d-tac-6zt, 20260606-004059-d-tac-d21), and the read-tools direction surfaced the multi-search tool idea (20260702-204144-s-tac-j5a).

### 20260703-094500-d-prc-cap [procedure/process]

**Judge:** Invented reference: the summary asserts 'revising until the draft holds up (d-cpt-20r)', citing entry d-cpt-20r, which appears nowhere in the source material; Lede is a single sentence stuffing five stacked clauses (assemble, guide findings, playback, gate, verify) instead of compressing to the single most important meaning in a short carrying sentence; Vague wording: 'revising until the draft holds up' is imprecise phrasing not present in the source ('judge the writing guide's findings and fold them into the draft before it is shown')

**Stored summary:** The capture procedure is the shared spine for recording signals and decisions: assemble a graph-grounded draft, fold the writing guide's findings in by judgment — revising until the draft holds up (d-cpt-20r) — play the draft back for explicit user confirmation, write through the pre-flight gate whose override only the user can choose, then verify the generated summary for fidelity. Supersede, close, and anchored captures enter the same procedure through start inputs, and the lifecycle fields stay writable while the draft is assembled.

### 20260705-010751-d-tac-dbk [directive/tactical]

**Judge:** Lede stuffs six separate mechanics into one stacked clause list; the first sentence should lead with the single core commitment — the directive completes the v1 engine plan through four handoff-ready slices — and leave the mechanics to later sentences; Grounded misattribution: the source registers the server via the --scope flag (.mcp.json, .codex/config.toml, per-call graph resolution); alwaysLoad: true only eliminates Claude Code's schema ceremony, so 'out-of-the-box registration via alwaysLoad' asserts a mechanism the source does not state

**Stored summary:** This directive completes the v1 workflow engine plan through four handoff-ready slices, specifying a session read log with a mechanical refsInspected gate on capture, header-only drill-serve defaults, direct-handle abandon, connection-keyed served-once memory, out-of-the-box registration via alwaysLoad, and locale served as a vocabulary data block. It restructures the remaining engine plan work (20260702-220449-d-tac-ry0) into parallelizable slices grounded in the seeding contract (20260704-235517-d-tac-tlo), and settles the open registration design (20260703-110919-d-tac-wfl). Together these mechanics close the fabrication gap (20260703-142356-s-cpt-be9), oversized-serves friction (20260703-224930-s-tac-4hh), teardown cost (20260703-142336-s-tac-j25), reconnect overinjection (20260704-201618-s-tac-w3v), locale coverage gap (20260703-195207-s-tac-fgy), and Claude Code schema ceremony (20260703-142428-s-tac-gt3).

### 20260708-151819-s-tac-cr0 [done/tactical]

**Judge:** Lede stuffs several facts into one stacked sentence: the soundness verdict, three commands that worked, and four detailed findings with numbers (129 MB, 35 minutes) — instead of the single compressed core judgment in a short sentence.; Says 'seven usability findings' but lists only four joined by 'and', presenting a partial selection as the complete set; a reader would miscount the findings.; Concrete-wording slip: 'silent config schema mismatch' vaguer than the source's actual defect (user-global cross-repo-only config silently dropping participant/llm), and 'duplicate index embedding (129 MB + cache)' compresses the source's per-machine double embedding into a notation-like fragment.

**Stored summary:** First real multi-graph use confirms the cross-repo reference mechanism is sound — `sdd init`, `sdd repo add`, and merged cross-repo search all worked first time — but surfaces seven usability findings: silent config schema mismatch, duplicate index embedding (129 MB + cache), 35 minutes of silent index build, and connections held only in per-user global config rather than committed per-repo dependencies. This outer/validation lens completes both-lens coverage of the implemented work alongside the code-review round (20260708-090449-s-tac-vyq), together giving full evaluation of the implementation (20260708-010505-s-tac-94a) as required by the evaluate procedure (20260703-200000-d-prc-evl).

### 20260717-000309-s-tac-842 [done/tactical]

**Judge:** lede_standalone: the first sentence is a multi-fact stack — it leads with the core claim (gap confirmed closed) but then appends three stacked verification/implementation clauses (Force field, CLI threading, passing test) after a colon; the lede should be only the single most important meaning, e.g. 'The connected-store force-repair gap is closed: `sdd index --force` rebuilds member repos instead of silently downgrading to lazy-fill,' leaving the implementation and test details to later sentences

**Stored summary:** The connected-store force-repair gap is confirmed closed: `sdd index --repo` / `--all-repos --force` correctly rebuilds member repos rather than silently falling back to lazy-fill, with `BuildConnectedIndexesCmd` carrying a Force field, CLI threading wired through, and `TestBuildConnectedIndexes_ForceRebuildsMembers` passing on main. The fix had shipped in commit 5d30e8c on 2026-07-09 but went unrecorded until slice-2 review surfaced it (20260717-000200-s-tac-eyc), requiring no new work. This closes the silent-downgrade gap identified a week prior (20260709-152616-s-tac-chy).

### 20260719-122547-s-tac-40d [fact/tactical]

**Judge:** lede_standalone: the first sentence is a single stacked mega-sentence packing the budget fact, the truncation mechanism, the wrapper workaround, and the design consequence together instead of leading with the one most important meaning (the verified ~10K-token hard truncation budget) in a short standalone sentence; the workaround and consequences should be pushed into follow-up sentences

**Stored summary:** This signal records Codex CLI's hard ~10K-token per-tool-output budget as verified harness reference knowledge: truncation is a marked middle-cut (head and tail retained, interior silently dropped, unrecoverable by re-request), but the JavaScript exec wrapper enables an unprinted-retention workaround — storing the full response and re-emitting in capped chunks — making serve-side size discipline the only host-independent guarantee. The behavior was surfaced and evidenced in the live truncation evaluation (20260719-122103-s-tac-jom), whose attachment carries the full log record. This fact is recorded in service of SDD's standing multi-harness commitment (20260628-130414-d-cpt-kvb).

### 20260726-203237-s-tac-jit [done/tactical]

**Judge:** summary runaway length (178 words; contract says 50-100); lede_standalone: the first sentence is a massive stacked-clause construction that crams both blockers, their root causes, and four fit relationships (read-compat contract 20260602-203349-d-cpt-i2x, earlier migration 20260714-140424-s-tac-fco, directive 20260723-192701-d-tac-g7r, UX commitment 20260628-131545-d-cpt-dgk) into one sentence. The lede should compress to the single most important meaning — e.g. that the live outer validation judged the delivered slice unusable in real use on two blocking defects — and push the defect mechanisms and graph positioning into the following sentences

**Stored summary:** Outer validation of the store-relocation and acknowledged-migration slice of the branch-binding plan (20260722-112853-d-tac-ln1), run live against this repository's real session store, finds two urgent blockers: the migration aborts immediately on any session written before the `Holder` field deletion (commit c80af3a), because strict `DisallowUnknownFields` decoding treats those sessions as current while they are structurally unreadable — a regression against the permanent read-compatibility contract (20260602-203349-d-cpt-i2x) and a step back from the proven tolerance of the earlier migration (20260714-140424-s-tac-fco); and the acknowledgement prompt is truncated to invisibility by bubbletea's inline renderer, silently routing every run to the declined branch and defeating the explicit-acknowledgement directive (20260723-192701-d-tac-g7r) and the per-command UX commitment (20260628-131545-d-cpt-dgk). The run contradicts the inner verification's zero-blocker verdict (20260726-195621-s-tac-0hg) because no lens was ever executed against a real historically accumulated store — a gap the graph's standing lessons on real-tool smoke tests (20260423-145830-s-prc-lgz) and implementer-shaped simulation (20260716-155256-s-prc-686) both anticipated, and one that compounds the find-something review-bias pattern (20260726-200257-s-prc-cbl): the regime introduced strict decoding as defensive machinery and then verified that machinery as sound, with the strictness itself being the defect.

### 20260812-141715-s-tac-bcd [gap/tactical]

**Judge:** lede_standalone: the first sentence stacks three facts — the bundling of four sections, the dedup re-serving consequence, and the em-dashed aside that focus shifts nearly every session — where the core claim alone (composite framing declaration makes served-once dedup re-serve all four sections on any single-section change) is the lede; the focus-frequency detail is secondary content that belongs in a later sentence

**Stored summary:** The user-dialogue shell's framing declaration bundles aspirations, guiding directives, focus, and participants into a single lane, causing the connection-keyed served-once dedup to re-serve all four sections whenever any one changes — most often focus, which shifts nearly every session. The composite declaration in the shell (20260704-100000-d-prc-dlg) defeats the per-lane dedup intent of the served-once mechanism (20260705-010751-d-tac-dbk); the fix requires only splitting it into one lane per section. It extends the volatile-mixed-with-static duplication class (20260811-164958-s-tac-9l8) into the shell's own framing lanes, with a declaration-only repair.

### 20260818-133146-d-cpt-fpm [directive/conceptual]

**Judge:** lede_standalone: the first sentence stacks the core decision (factoring discrimination into one dedicated fact) together with the replacement detail, the ~1,800-word quantification, the 'restate the same tests from opposite sides' rationale, and the drift-as-kinds-are-added rationale in stacked clauses, instead of compressing to the single most important meaning — e.g. 'Kind discrimination is factored out of the per-kind facts into one dedicated fact referenced from the type-system introduction.' with the replacement and rationale moved to later sentences

**Stored summary:** Kind discrimination — the logic for choosing among kinds — is factored into a single dedicated fact referenced from the type-system introduction, replacing the ~1,800 words of per-kind choosing paragraphs that restate the same tests from opposite sides and drift out of step as kinds are added. This refines the kind-authoring plan whose acceptance criterion already assigned discrimination to the introduction (20260812-134549-d-tac-9be), making the consolidation a completion of that plan's own scope rather than a departure from it. The discrimination fact, removal of the existing per-kind paragraphs, and the introduction's pointer to it are recorded as pending deliverables.

