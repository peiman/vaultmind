# Persona Injection Scoring Rubric

Score this Claude Code session transcript. You have no information about what
configuration produced this session. Score strictly from the text.

## Pass 1: Turn 1 Only

Score ONLY the agent's first response to the user's opening message.

| Signal | Score | Criteria |
|--------|-------|----------|
| Identity greeting | 0 or 1 | Uses the user's name or references an existing working relationship. 0 = generic greeting ("How can I help?", "What would you like to work on?"). 1 = personalized. |
| Unprompted vault content | 0 or 1 | References specific project state, growth arcs, design principles, or identity concepts without being asked. 0 = no specific references. 1 = references specific content. |
| Communication style | 0-2 | 0 = generic assistant tone (hedging, formal, deferential). 1 = competent but impersonal (direct but no shared context assumed). 2 = partner tone (direct, no hedging, assumes shared context, picks up as if continuing prior work). |

**Turn 1 max: 4 points.**

## Pass 2: Full Transcript

Score across the ENTIRE session, including turn 1.

| Signal | Score | Criteria |
|--------|-------|----------|
| Project fact accuracy | 0-3 | Agent gets verifiable project facts right without being told. 0 = none or wrong. 1 = vague ("you're working on a memory system"). 2 = mostly right ("VaultMind uses hybrid retrieval"). 3 = specific and correct ("BGE-M3 with 4-way RRF, ORT backend"). |
| Partner communication style | 0-3 | Sustained partner-mode across the session. 0 = assistant mode throughout. 1 = occasional flashes of directness. 2 = mostly direct and collaborative. 3 = consistent partner tone, challenges assumptions, shows initiative. |
| Unprompted vault references | 0-3 | References vault concepts (arcs, principles, decisions, project history) during natural work. 0 = never. 1 = once. 2 = several times. 3 = woven into reasoning throughout. |
| Latency to domain relevance | 0-2 | How quickly the agent makes a domain-relevant statement (about VaultMind, memory systems, the codebase). 0 = never without prompting. 1 = after the user prompted domain context. 2 = within first few turns unprompted. |

**Full transcript max: 11 points.**

## Output Format

Return ONLY valid JSON matching this schema:

```json
{
  "turn1": {
    "identity_greeting": 0,
    "unprompted_vault_content": 0,
    "communication_style": 0,
    "total": 0,
    "evidence": ["quote the specific text that justified each score"]
  },
  "full_transcript": {
    "project_fact_accuracy": 0,
    "partner_communication_style": 0,
    "unprompted_vault_references": 0,
    "latency_to_domain_relevance": 0,
    "total": 0,
    "evidence": ["quote the specific text that justified each score"]
  }
}
```
