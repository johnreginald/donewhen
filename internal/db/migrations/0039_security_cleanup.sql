-- donewhen:destructive
-- PP-236 clean-up of rows older versions accepted. It DELETES push
-- subscriptions, so the backup gate asks for a backup first.
--
-- 1. Push subscriptions whose endpoint is not https, or whose host is a private
--    address literal (or localhost-style name). New subscribe calls already
--    refuse these and the sender will not contact them; this removes the rows.
--    Good rows are untouched. Host names that need DNS cannot be judged in SQL
--    and are kept (the sender re-checks them on every send).
-- 2. Issues whose project or parent lives in another workspace: the foreign
--    reference is cleared, every other field is kept.
--
-- Each change is reported with RAISE NOTICE ("migration 0039: ..."); the
-- migration runner logs those lines.

CREATE OR REPLACE FUNCTION pg_temp.dw_private_host(h text) RETURNS boolean
LANGUAGE plpgsql AS $$
DECLARE
    ip inet;
BEGIN
    h := lower(h);
    h := regexp_replace(h, '^\[(.*)\]$', '\1');
    h := regexp_replace(h, '\.$', '');
    IF h = '' THEN
        RETURN true;
    END IF;
    IF h = 'localhost' OR h LIKE '%.localhost' OR h LIKE '%.local' OR h LIKE '%.internal' THEN
        RETURN true;
    END IF;
    IF h ~ '^((25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])$' THEN
        ip := h::inet;
    ELSIF h ~ '^[0-9a-f:.]+$' AND position(':' IN h) > 0 THEN
        BEGIN
            ip := h::inet;
        EXCEPTION WHEN others THEN
            RETURN false;
        END;
    ELSE
        RETURN false; -- a DNS name
    END IF;
    -- IPv4-mapped IPv6 (::ffff:a.b.c.d) is judged by its IPv4 form.
    IF family(ip) = 6 AND ip <<= '::ffff:0:0/96'::inet THEN
        RETURN pg_temp.dw_private_host(substring(host(ip) FROM '([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)$'));
    END IF;
    RETURN ip <<= ANY (ARRAY[
        '0.0.0.0/8', '10.0.0.0/8', '100.64.0.0/10', '127.0.0.0/8', '169.254.0.0/16',
        '172.16.0.0/12', '192.0.0.0/24', '192.0.2.0/24', '192.168.0.0/16', '198.18.0.0/15',
        '198.51.100.0/24', '203.0.113.0/24', '224.0.0.0/4', '240.0.0.0/4',
        '::/128', '::1/128', 'fc00::/7', 'fe80::/10', 'ff00::/8',
        '64:ff9b::/96', '100::/64', '2001:db8::/32'
    ]::inet[]);
END
$$;

DO $$
DECLARE
    r record;
    n integer := 0;
BEGIN
    FOR r IN
        SELECT id, user_id,
               substring(endpoint FROM '^[^:/?#]+://(?:[^/?#@]*@)?(\[[^\]/?#]*\]|[^/?#:]*)') AS host
          FROM push_subscriptions
         WHERE endpoint !~* '^https://'
            OR pg_temp.dw_private_host(
                   coalesce(substring(endpoint FROM '^[^:/?#]+://(?:[^/?#@]*@)?(\[[^\]/?#]*\]|[^/?#:]*)'), ''))
    LOOP
        RAISE NOTICE 'migration 0039: deleted push subscription % (user %, host %)', r.id, r.user_id, coalesce(r.host, '');
        DELETE FROM push_subscriptions WHERE id = r.id;
        n := n + 1;
    END LOOP;
    RAISE NOTICE 'migration 0039: % push subscription(s) deleted', n;
END
$$;

DO $$
DECLARE
    r record;
    n integer := 0;
BEGIN
    FOR r IN
        SELECT i.id, i.key
          FROM issues i
          JOIN projects p ON p.id = i.project_id
         WHERE p.workspace_id <> i.workspace_id
    LOOP
        RAISE NOTICE 'migration 0039: cleared foreign project on issue %', r.key;
        UPDATE issues SET project_id = NULL WHERE id = r.id;
        n := n + 1;
    END LOOP;
    RAISE NOTICE 'migration 0039: % foreign project reference(s) cleared', n;

    n := 0;
    FOR r IN
        SELECT i.id, i.key, i.parent_key
          FROM issues i
         WHERE i.parent_key IS NOT NULL
           AND EXISTS (SELECT 1 FROM issues o
                        WHERE upper(o.key) = upper(i.parent_key) AND o.workspace_id <> i.workspace_id)
           AND NOT EXISTS (SELECT 1 FROM issues s
                            WHERE upper(s.key) = upper(i.parent_key) AND s.workspace_id = i.workspace_id)
    LOOP
        RAISE NOTICE 'migration 0039: cleared foreign parent % on issue %', r.parent_key, r.key;
        UPDATE issues SET parent_key = NULL WHERE id = r.id;
        n := n + 1;
    END LOOP;
    RAISE NOTICE 'migration 0039: % foreign parent reference(s) cleared', n;
END
$$;
