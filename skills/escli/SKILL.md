---
name: escli
description: Read data from Elasticsearch with the escli command-line client. Use when the user asks about data stored in Elasticsearch or Kibana, such as listing indices, looking up fields, searching documents, counting, aggregating or running ES|QL queries.
---

# Reading Elasticsearch with escli

escli is a read-only Elasticsearch client. It prints JSON to stdout and errors
to stderr, with exit code 1 on failure.

## Before you start

Run `escli auth status`. If it says escli is not set up, or that the API key is
wrong or expired, stop and ask the user to run `escli init` in their own
terminal. Never ask for, read or handle the API key yourself.

## Explore first, then query

1. `escli indices` lists indices, aliases and data streams. Add a pattern such
   as `'deals*'`, or `--stats` for document counts.
2. `escli mapping <index> --flat` lists each field and its type. Check field
   names here before you write a query; do not guess them.
3. Query with one of the commands below.

## Queries

- ES|QL is usually the best choice for filtering, aggregating and sorting:
  `escli esql 'FROM deals | WHERE amount > 10000 | STATS n = COUNT(*), total = SUM(amount) BY stage | SORT total DESC'`
  It returns at most 1000 rows unless the query sets `LIMIT`.
- Lucene search for documents:
  `escli search deals -q 'stage:closedwon AND owner:pat*' --size 5 --fields name,amount,stage`
- A full search request, for example with aggregations:
  `escli search deals --size 0 --body '{"aggs":{"by_stage":{"terms":{"field":"stage.keyword"}}}}'`
- `escli count deals -q '...'` and `escli get <index> <id>`.
- Any other read endpoint: `escli api GET /_cluster/health`.

## Rules

- Use `text` fields for full-text matching, and their `.keyword` sub-field for
  exact matches, sorting and terms aggregations.
- Keep `--size` small and use `--fields` to limit the output.
- Use `--compact` when the output is long.
- Do not pass `--allow-write` unless the user explicitly asked you to change
  data in Elasticsearch.
