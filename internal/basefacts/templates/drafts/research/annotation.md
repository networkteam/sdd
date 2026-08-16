# Research brief — the annotation signal kind

## A. Semantics
- A1: Its question: "What structure lies over other entries?" (overview.go — generated; do not restate).
- A2: Structural metadata layered onto the entries it references, with topics as one annotation form (d-tac-n0u; annotation_capture.tmpl; framework-concepts:27).
- A3: Structural, not observational; exists to be QUERIED for clustering, not read as observations (annotation_capture.tmpl; annotation.go: excluded from catch-up narrative rendering, surfaces via topic queries). Pre-flight routes annotation as a structural check capped at low findings.
- A4: The kind is general; topics are today's only form — future forms (severity tagging, dependency-type tagging) can add frontmatter fields without a new kind (d-tac-n0u). AnnotationFields currently has exactly one field: Topics.
- A5: Membership is binary, never weighted — "topics stay binary membership"; gradation is heat's domain (d-cpt-5nn).
- A6: What an annotation produces is membership, indistinguishable from inline tagging — EffectiveTopics unions inline topics with annotation-declared ones; "an entry is 'in topic X' whether it tagged itself or some annotation tagged it" (focus.go; topic_filter.go).
- A7: An annotation is itself a member of every label it declares; members sub-selections narrow which refs receive a label, never whether the annotation carries it (focus.go; d-tac-6tz AC 3).
- A8: An open annotation demands nothing — deliberately not an attention kind; status open indefinitely is the correct resting state.
- A9: Retirement/re-labeling: replacement annotation (rare; usually a new annotation instead) or directive retiring it. KEY LIMITATION: superseding to re-label "leaves the old label attached: inline cases are untouched and the old annotation keeps declaring it unless separately retired, so effective topic sets accumulate stale labels" (s-cpt-k1b); "No alias mechanism — drift is addressed by new annotations, not rewriting history" (d-cpt-7iy).
- A10: The annotation never touches the entries it labels — "adds the membership rather than re-tagging the immutable original" (s-cpt-axw live exemplar; immutability d-cpt-e1i).

## B. Make-up (Go names, validateAnnotation in construction.go)
- At least one ref: "annotation signal must carry at least one ref (the entries the annotation is about)".
- At least one topic: "annotation signal must declare at least one topic".
- Every label parses as a topic path (ParseTopicPath).
- Mapping form requires label.
- members ⊆ refs: "topics[i].members[j]: <id> is not in refs (members must be a subset of the annotation's refs)".
- No inline topics on an annotation: "inline topics are not valid on kind: annotation signals (topics carry the annotation's assignments)".
- Annotation topics only on annotations; at-most-one-block agreement.
- Go names: KindAnnotation; AnnotationFields{Topics []AnnotationTopic}; AnnotationTopic{Label, Members} (marshals to compact scalar form when Members empty); Entry.AnnotationTopics vs Entry.Topics mutually exclusive by kind; IsAnnotation(); MembersFor(); Graph.Annotations(); EffectiveTopics().
- The two item forms (d-tac-n0u): string form (applies to all refs — "the default and most common case") and {label, members} object form (sub-selection).
- Refs carry a dual meaning on an annotation: the canonical member set for any label without its own members (d-tac-n0u — members deliberately not a separate edge field).
- Label grammar: /-joined components [letters digits -]; case-insensitive comparison, first-seen casing wins; topic() filtering is component-wise prefix match.
- Body: no requirement — "the body is allowed to be empty or very brief". No layer rule ("layer-flexible"); live exemplars all conceptual/medium but nothing declares it.
- MECHANICS NOTE: no exported constants exist for annotation rules — inline literals in validateAnnotation; extraction needed before a mechanics block can render them. FLAG for implementer.

## C. Craft claims
1. Reach for an annotation when the entry is already written and the label came later — "inline topic labels as the zero-ceremony primary path at capture; structural annotation signals for post-capture correction and bulk reorganization" (s-cpt-8h8, founding design).
2. Tag at capture when you can; annotate when you cannot (same; d-tac-zyd retired mass backfill because capture-time tagging already covered the active graph).
3. Never supersede an entry just to change its tags — add the membership externally (s-cpt-axw; immutability).
4. Reuse a label before minting one — "a label means the same cluster everywhere it appears; the graph stays coherent only if labels are reused rather than reinvented" (d-tac-6tz → framework-concepts:213).
5. Prefer hierarchical paths for families.
6. A new label is worth founding as a real cluster, not a singleton (s-cpt-ls5: "Captured so the new label is a real cluster rather than a singleton").
7. Research the existing vocabulary before proposing a label — look at what labels neighbours already carry (d-tac-6tz survey procedure, act-generalized).
8. One annotation can found a whole family in one act (the 2026-06-01 batch: one annotation per family, 3-17 refs each).
9. A single-ref annotation is legitimate — "one-off or bulk" (s-cpt-axw, s-cpt-hul; rubric).
10. The body's job is to name the cluster, not argue anything — every live exemplar opens with the cluster name and a one-line gloss.
11. When one label covers only some refs, say why those (rubric 2).
12. Label evolution costs more than getting it right once — the old label survives (s-cpt-k1b).

## D. Reverse side (annotation_capture.tmpl — low-only)
- D1: A label specific enough to discriminate against future captures; multi-component paths preferred; misc/general/other/tmp will not support filtering.
- D2: A sub-selection briefly indicates why those entries were singled out.
- D3: A terse or empty body is correct, never a defect. Prose density is not a virtue in this kind.
- D4: One-off and bulk equally correct.
- D5: Layer is a free choice.
- D6: Structural rules are mechanical, not judgment.
- D7: Nothing about an annotation rises to a blocking concern.

## E. Discriminators
- E0: OVERVIEW OWNS "An annotation carries structure, not narrative — metadata laid over the entries it references" — go PAST it, not repeat it.
- E1 vs every narrative kind: a gap/fact/question/insight/done each adds a proposition; an annotation adds no proposition — an index over propositions already there. TEST: strip the refs away — if anything is still worth reading, it is not an annotation.
- E2 vs inline topics: inline is the primary zero-ceremony path at the moment of writing; an annotation is the path when that moment has passed — the entry is immutable, the label didn't exist yet, or one act should re-thread many entries. Not two mechanisms with different meanings — the same membership, reached at a different time.
- E3 vs superseding the tagged entry: never the move.
- E4 vs focus: a focus commits attention (who, what, this period); an annotation commits nothing and prioritizes nothing.
- E5: An annotation never closes anything; a directive retires an annotation.
- E6 non-software instantiation (timber shop): three separate entries over two months — a delivery arrived C16 against a C24 spec; a customer reported a sagging purlin; the shop decided to change supplier. Nobody had a word for the thread when each was written. An annotation pointing at all three declaring `timber/stock-quality` makes them one pull-able cluster without rewriting anything. It still discriminates: remove it and the shop knows exactly as much as before — it just cannot find the three in one pull. If the shop wanted to record that C16 keeps arriving against spec, that is a gap. Sub-selection: only the delivery and supplier-change also belong to `suppliers/sourcing` — {label, members} with the body saying why the sagging report is out.

## F. Contradictions / rulings
- F1: "surface only via topic queries" (code comment) vs shipped behavior (annotations appear in list/filter paths and show renders Topics). Only the NARRATIVE exclusion is intact as intent.
- F2: The narrative exclusion is asserted (d-cpt-7iy) but not enforced anywhere — an open AC on d-tac-9be covers it.
- F3: The {label, members} sub-selection form is UNEXERCISED in the entire live graph — every live annotation declares all labels over all refs while the prose describes sub-groupings the structure doesn't express. RULING NEEDED: drafting defect worth naming as an anti-pattern, or accepted shape?
- F4: Ref-kind on annotation member refs: every live annotation uses related (or legacy unknown) because refs are the structural member set, not a semantic relationship — but ref-kind is required at capture and no source says which to pick. RULING NEEDED.
- F5: Engine capture still has no annotation branch (s-tac-6rd open; construction side landed, engine draft carrier missing — an engine-drafted annotation today fires two findings). Carried as an open AC on d-tac-9be. Don't state engine capability the engine lacks.
- F6: overview promises "each kind has its own authoring fact" — wiring must add the ref + registry entry.
- Unsourced (flag, don't blend): "asserts nothing about the world" as literal phrasing (nearest: "structural, not observational"); layer convention (all live conceptual — nothing declares it); confidence convention (all medium — no source); summary shape conventions.

## G. Overview coverage — point, don't restate
The kind question; the structure-not-narrative line; signal/decision split; immutability; retirement split incl. same-kind supersession; layers; the per-kind fact promise. NOT in the overview (the fact's own ground): the two topics item forms + members⊆refs; no-inline-topics exclusivity; refs-as-member-set; effective-topic merge + self-membership; label stability/reuse; the inline-vs-annotation timing discriminator; the empty-body license; binary membership; the no-alias / stale-label limitation.
