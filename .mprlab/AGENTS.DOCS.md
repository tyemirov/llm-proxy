# AGENTS.DOCS.md

## Scope

This guide controls English technical documentation in this repository.

Use ASD-STE100 Simplified Technical English, Issue 9, January 2025, for all new technical prose.

Use the same standard for all technical prose that you change.

This rule applies to these documents:

- Product requirement documents (PRDs).
- Architecture documents.
- Issue trackers and issue bodies.
- Plans, policies, and architecture decision records (ADRs).
- README files, runbooks, API documents, and agent guides.

Do not change code, commands, paths, identifiers, URLs, quoted text, proper names, legal text, or third-party text.

Do not change a fact, requirement, interface, or ownership boundary to make the language simpler.

## Official Standard

Use the official [ASD-STE100 Issue 9](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf) as the primary source.

The standard has writing rules and a controlled dictionary. Use the two parts during the language review.

Run the skill `prepare-ste-reference` script before you write or revise technical prose.

The script caches the official PDF outside the repository and verifies its pinned SHA-256.

Use Part 1 for the writing rules. Use Part 2 as the controlled dictionary.

The producing agent owns the final language review. Do not give this work to the end user.

If the script fails, stop the documentation work and report the source blocker. Do not use an unverified copy.

Do not commit, publish, or redistribute the cached PDF or its dictionary.

## Repository Terms

Read `.mprlab/TERMINOLOGY.md` before you write.

Use an approved dictionary word only with its approved meaning and part of speech.

Use a repository term only as an approved technical noun or technical verb.

Add a necessary repository term to `.mprlab/TERMINOLOGY.md`. Give the term one meaning and one part of speech.

Use the same term for the same item, action, state, or interface. Do not use a different synonym.

Keep the exact spelling of names and source-controlled literals.

## Procedures

Use procedural writing for instructions, requirements, acceptance criteria, validation steps, and issue deliverables.

- Use the imperative form.
- Write one instruction in each sentence.
- Use a maximum of 20 words in each sentence.
- Put a condition before the instruction when the reader must know the condition first.
- Use a vertical list for a complex sequence or a set of related instructions.
- Use a note only for information. Do not put an instruction in a note.

## Descriptions

Use descriptive writing for product context, architecture, decisions, behavior, and status.

- Use a maximum of 25 words in each sentence.
- Give one subject or idea in each sentence.
- Give information gradually.
- Start each paragraph with its topic.
- Keep related information in one paragraph.
- Use a maximum of six sentences in each paragraph.

## General Writing Rules

- Use approved words, approved technical nouns, and approved technical verbs.
- Use American English spelling.
- Use the active voice. Use the passive voice only when the agent is not known in descriptive text.
- Use simple present, simple past, or simple future verb forms.
- Do not use a progressive or perfect verb form.
- Use an `-ing` form only in an approved word or a technical noun.
- Use a direct verb to describe an action.
- Do not use a phrasal verb unless the dictionary approves its exact meaning.
- Keep a multi-word noun to three words or fewer.
- Use articles where they are necessary.
- Do not use contractions.
- Do not use semicolons.
- Use `must` for a binding requirement. Do not use `shall` or `should`.
- Use `can` for capability. Do not use `may` for capability.
- Use consistent terminology and sentence patterns.

## Document Use

- Write PRD and architecture context as descriptive text.
- Write PRD requirements and acceptance criteria as procedures.
- Write an issue goal as descriptive text.
- Write issue requirements, deliverables, and validation as procedures.
- Write a runbook step as a procedure.
- Write an ADR decision and its results as descriptive text.
- Write a changelog entry in the simple past tense.

## Production Sequence

1. Run the skill `prepare-ste-reference` script and get the verified PDF path.
2. Read the source facts and the current repository contract.
3. Identify each section as procedural or descriptive.
4. Identify the necessary technical nouns and technical verbs.
5. Add new approved terms to `.mprlab/TERMINOLOGY.md`.
6. Write the document without a change to its technical meaning.
7. Run the skill `check-ste` script on each document that you changed.
8. Review the document against all applicable rules in Part 1.
9. Review each general word against Part 2 for its meaning and part of speech.
10. Correct each language error before you report completion.

## Review Gate

The producing agent must complete this gate before it reports ASD-STE100 compliance.

Make sure that:

- Each general word is approved for its meaning and part of speech.
- Each project term is an approved technical noun or technical verb.
- Each procedural sentence has 20 words or fewer.
- Each descriptive sentence has 25 words or fewer.
- Each instruction contains one action, unless the actions occur at the same time.
- Each sentence uses an approved verb form.
- Each sentence uses active voice, unless the descriptive exception applies.
- Each paragraph has one topic and a maximum of six sentences.
- The document has no semicolon, contraction, unapproved synonym, or ambiguous pronoun.
- The simplified text has the same technical meaning as its source.

If you did not review all applicable text, report the exact scope that you reviewed. Do not claim full-document compliance.

Never ask the end user to complete a rule check, dictionary check, or compliance decision.
