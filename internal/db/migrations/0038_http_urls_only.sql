-- Stored links are rendered as hrefs, so only absolute http(s) URLs with a host
-- may stay. Anything else (javascript:, data:, a bare string) was accepted by
-- older versions; clear it. Only the offending link is nulled, no row is deleted.
UPDATE issues        SET pr_url   = NULL WHERE pr_url   IS NOT NULL AND pr_url   !~* '^https?://[^/?#[:space:]]+';
UPDATE issue_commits SET url      = NULL WHERE url      IS NOT NULL AND url      !~* '^https?://[^/?#[:space:]]+';
UPDATE projects      SET repo_url = NULL WHERE repo_url IS NOT NULL AND repo_url !~* '^https?://[^/?#[:space:]]+';
UPDATE initiatives   SET repo_url = NULL WHERE repo_url IS NOT NULL AND repo_url !~* '^https?://[^/?#[:space:]]+';
