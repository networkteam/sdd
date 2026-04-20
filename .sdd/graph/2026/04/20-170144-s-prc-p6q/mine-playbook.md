# Mine playbook move — session evidence and sketch

## What we did this session

Source: podcast transcript (Simon Willison on Lenny's Podcast, ~90 min)

**Steps followed:**
1. User provided transcript file path
2. Claude read the transcript in chunks, identified ~6 candidate touchpoints with the graph
3. Presented candidates grouped by theme; user confirmed which resonated (3 out of 6 strong, 1 noted as already covered, 1 as future aspiration)
4. For each confirmed signal: played back proposed entry, user adjusted, ran `sdd new` with verbatim quotes attached and source URL in description
5. Result: 3 signals captured (`s-stg-jbo`, `s-cpt-l2q`, `s-stg-ob9`) + this gap signal

**What made it work:**
- Dialogue as filter — user decided what mattered; Claude didn't bulk-import
- Touchpoint identification before capture — efficient, skipped irrelevant material
- Verbatim quotes in attachment — preserves original meaning, prevents paraphrase drift
- Source URL in description — provenance permanently traceable
- Separate plays for distinct insight layers (strategic vs conceptual)

## Sketch of a "mine" playbook move

**Trigger:** User provides an external source — transcript file, URL, document, set of notes.

**Steps:**
1. **Read** — ingest the source (chunked if large); build internal summary
2. **Identify touchpoints** — surface candidate connections to the current graph: open signals it speaks to, decisions it validates or challenges, new ideas not yet in the graph. Present as a numbered list, one sentence each
3. **Dialogue** — walk through candidates with user. "Does this resonate? Already covered? Worth a signal?" User drives prioritization
4. **Capture** — for each confirmed item: play back the proposed entry (type, layer, refs, description), get confirmation, run `sdd new` with verbatim source quotes as attachment and source URL in description
5. **Optional close** — capture a done signal noting the source was mined, linking to captured entries

**Conventions:**
- Always attach verbatim quotes from the source — never paraphrase alone
- Include source URL in the entry description
- Confidence reflects how well the insight is validated: `high` if source is authoritative and quote is direct; `medium` for interpretation
- Keep the filter tight: 3 sharp signals > 10 weak ones
