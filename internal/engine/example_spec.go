package engine

// ExampleSpecFrontmatter is the worked example the procedure spec reference
// fact serves: a small review move written for width — optional param and
// optional collect, two injects with differently shaped args, multi-clause
// infix guards, a user chooser looping back for correction, a dispatch seed
// on the confirming option, and a closing gate on the field the dispatched
// recording hands back. Authored beside the engine so its own tests load it —
// the served example cannot drift from what the engine accepts.
const ExampleSpecFrontmatter = `params:
    anchorHint: {type: text, optional: true, desc: "the user's pointer to what is being reviewed, in their words"}
state:
    anchor: {type: entry-id, optional: true, desc: "the entry the review centres on, when one exists"}
    widenReport: {type: text, desc: "the searches run and sources read, each with what it surfaced"}
    inspectedIds: {type: list<entry-id>, desc: every entry read in full that the review will reference}
    brief: {type: text, desc: "the account of what happened, tied to its evidence"}
    synthesis: {type: text, desc: "the reading of why, stated so its confidence and limits are visible"}
    doneEntry: {type: entry-id, desc: the recorded review the dispatched capture hands back}
steps:
    - id: scope
      inject:
          - {fn: entryChains, args: {up: 2, down: 1}}
          - {fn: viewLayout, args: {layout: 'active:as-list:name("Active work")', maxBytes: 2000}}
      collect: ["anchor?", widenReport, inspectedIds]
      transitions:
          - when: hasWidenReport and hasInspectedIds and inspectedIdsResolve
            to: account
    - id: account
      collect: [brief, synthesis]
      transitions:
          - when: hasBrief and hasSynthesis
            to: review
    - id: review
      chooser: user
      options:
          - choice: confirm
            dispatch:
                procedure: capture
                seed:
                    widenReport: widenReport
                    anchor: anchor
            to: record
          - {choice: correct, collect: ["brief?", "synthesis?"], to: account}
          - {choice: abandon, to: end(abandoned)}
    - id: record
      collect: [doneEntry]
      transitions:
          - when: hasDoneEntry and doneEntryResolves
            to: end(completed)
`
