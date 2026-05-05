# Skill modularization research — verified sources

2026-05-05 literature scan conducted via Perplexity, then independently verified by URL fetch and content check. Sources are categorized by quality below; Perplexity's original output included weak sources and one outright misattribution that have been filtered out of the kept set.

## Strong primary sources (kept)

### ManyIFEval / "Curse of Instructions"

- Harada et al., University of Tokyo (Matsuo Lab), ICLR 2025 submission
- https://openreview.net/forum?id=R6q67CDBCH
- Demonstrates a power-law decline in compliance as the number of simultaneous verifiable instructions grows, on frontier models (Claude, GPT-4-class, Gemini, open-source).

### When Instructions Multiply

- Harada, Yamazaki et al. (same Matsuo Lab), arxiv preprint
- https://arxiv.org/abs/2509.21051
- Follow-on work: introduces ManyIFEval + StyleMBPP, regression models for predicting degradation. Direct evidence of multi-instruction degradation.

### AgentIF

- Qi, Peng, Wang, Hou, Li (Tsinghua NLP), arxiv preprint
- https://arxiv.org/abs/2505.16944
- 707 human-annotated agentic instructions averaging 1,723 words and ~12 constraints each. Reports degradation as instruction length increases in agentic settings.

### Lost in the Middle (canonical)

- Liu et al. 2023, arxiv
- https://arxiv.org/abs/2307.03172
- The original positional-bias paper. Foundational citation for any lost-in-the-middle claim.

### Lost-in-the-Middle in Long-Text Generation

- Zhang et al., arxiv preprint (2025)
- https://arxiv.org/abs/2503.06868
- 2025 follow-on study confirming the U-shaped positional bias persists in long-text generation. Note: preprint, not yet peer-reviewed; on-topic for current-evidence claims.

### ReasonIF

- Kwon, Zhu, Bianchi, Zhou, Zou (Stanford-affiliated, James Zou's group), arxiv preprint
- https://arxiv.org/abs/2510.15211
- Reasoning-mode instruction adherence as a separate concern; success rates below 0.25 on some benchmarks. A different failure mode than raw count or position.

## Vendor documentation (kept)

### Anthropic Skills documentation

- https://support.claude.com/en/articles/12512198-how-to-create-custom-skills
- Official Anthropic primary source. Explicitly defines progressive disclosure: "The metadata in the SKILL.md file serves as the first level of a progressive disclosure system, providing just enough information for Claude to know when the Skill should be used without having to load all of the content." Direct vendor recommendation for the modular pattern.

## Sources dropped from Perplexity output

These appeared in the original Perplexity result but failed source-quality verification:

- `gist.github.com/0xfauzi/...` — anonymous GitHub gist, off-topic for the attributed claim (it's an AGENTS.md best-practices guide, not instruction-density evidence). **Drop.**
- `aihero.dev/a-complete-guide-to-agents-md` — unbylined industry blog; secondary at best, doesn't actually compare Anthropic vs OpenAI modular design as Perplexity claimed. **Drop.**
- `dev.to/gabrielanhaia/...` — practitioner blog post summarizing Liu et al. 2023; readable synthesis but not citable as primary evidence. **Drop, cite the underlying papers directly.**
- `arxiv.org/abs/2511.13900` — small-model preprint with limited authority; borderline-relevant but weak as headline 2025–2026 evidence. **Drop.**

## Perplexity misattribution

- `arxiv.org/abs/2502.14255` — Perplexity attributed this URL to "ManyIFEval / instruction-count degradation," but the paper is actually titled *"Effects of Prompt Length on Domain-specific Tasks for Large Language Models"* (Liu, Wang, Willard) and concerns financial sentiment / monetary policy tasks. **Wrong paper entirely.** This was a Perplexity citation hallucination. The lesson: always verify Perplexity citations before quoting.

## Stronger replacements not in original Perplexity output

- For lost-in-the-middle as foundational evidence: **Liu et al. 2023, arxiv 2307.03172** is canonical and should always be cited first (added above).
- For positional-bias 2025+ benchmarks: **RULER** (Hsieh et al. 2024) and **HELMET** are recognized long-context benchmarks worth investigating for stronger headline evidence than the small preprints Perplexity surfaced. Not yet verified directly; flagged for follow-up if stronger current-evidence is needed.

## Evidence quality summary

**Strong:**

- Multi-instruction degradation is empirically measured (ManyIFEval, AgentIF) — solid finding from venue-submitted and well-affiliated work.
- Lost-in-the-middle persists on long-context models — canonical paper plus follow-on work confirms.
- Vendor architectures explicitly favor modular, on-demand loading — Anthropic's docs are unambiguous.

**Weaker / mostly anecdotal:**

- Exact size thresholds ("10k tokens is too long," "split at 5k tokens") have no published basis.
- Universal claims that monolithic prompts are always inferior overstate the research; the support is for modular-by-default with routing care, not modular-as-prerequisite.
- Claims that 1M-context models have "solved" positional bias are not supported.

The current research supports a **modular-by-default** design, but with careful routing reliability and per-case empirical validation rather than blind fragmentation.
