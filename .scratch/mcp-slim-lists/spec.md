# Slim MCP list responses

## Objective
Cut tokens Raenil MCP spends per call. Measured on prod 2026-09-24: `list_issues {}` 3.0 MB (1178 rows, no limit),
`list_issues {limit:20}` 66 KB, `list_documents` 912 KB, `list_issues_missing_docs` 1.1 MB, `list_projects` 104 KB.
Cause: list rows carry full markdown bodies, full label objects, uuids (id/workspaceId/stateId), indented JSON, no default limit.

## Scope (MCP layer only — REST/web/orchestrator untouched)
- list_issues / list_issues_missing_docs: slim rows {key,title,state name,priority,labels (names),parentKey,project,childCount,updated date}.
- list_documents: slim rows, no body; `issue` filter accepts key.
- list_projects: no description/timestamps.
- list_issue_labels: {id,name,group} — no color/groupId uuid.
- Default limit 50 on issue/doc lists; truncation note when more exist.
- `verbose:true` on those lists returns full objects (still limited).
- Compact JSON for every tool. get_* unchanged in content.

## Done when
- [x] 20 realistic issues through list_issues projection < 8 KB (test)
- [x] no body fields (descriptionMd/bodyMd) in slim list rows (test)
- [x] default limit + truncation note (test)
- [x] get_issue/get_document still return full objects
- [x] go build ./... && go test ./internal/mcp/... pass

## Result (prod payloads captured 2026-09-24, run through the new projection)
| call | before | after |
|---|---|---|
| list_issues {} | 3.0 MB | 11.6 KB (cap 50) |
| list_issues {state:Ready} | 471 KB | 12.0 KB |
| 20 issues | 37 KB (compact) / 66 KB (indented) | 4.8 KB |
| list_issues_missing_docs | 1.1 MB | 10.2 KB |
| list_documents | 918 KB | 10.9 KB |
| list_projects | 105 KB | 10.7 KB |
| list_issue_labels | 55 KB | 1.5 KB |
