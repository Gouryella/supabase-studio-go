package api

import (
	"net/http"
	"time"
)

func (api *API) handleRunLints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	query := enrichLintsQuery(lintSQL, "public, storage")
	body, pgErr, _, err := api.pgMetaExecute(r, query, false)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	if pgErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"message":        pgErr.Message,
			"formattedError": pgErr.FormattedError,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func enrichLintsQuery(query, exposedSchemas string) string {
	header := "set pg_stat_statements.track = none;\n"
	if exposedSchemas != "" {
		header += "set local pgrst.db_schemas = '" + exposedSchemas + "';\n"
	}
	header += "-- source: dashboard\n-- user: self host\n-- date: " + time.Now().UTC().Format(time.RFC3339) + "\n\n"
	return header + query
}

const lintSQL = `set local search_path = '';

(
with foreign_keys as (
    select
        cl.relnamespace::regnamespace::text as schema_name,
        cl.relname as table_name,
        cl.oid as table_oid,
        ct.conname as fkey_name,
        ct.conkey as col_attnums
    from
        pg_catalog.pg_constraint ct
        join pg_catalog.pg_class cl -- fkey owning table
            on ct.conrelid = cl.oid
        left join pg_catalog.pg_depend d
            on d.objid = cl.oid
            and d.deptype = 'e'
    where
        ct.contype = 'f' -- foreign key constraints
        and d.objid is null -- exclude tables that are dependencies of extensions
        and cl.relnamespace::regnamespace::text not in (
            'pg_catalog', 'information_schema', 'auth', 'storage', 'vault', 'extensions'
        )
),
index_ as (
    select
        pi.indrelid as table_oid,
        indexrelid::regclass as index_,
        string_to_array(indkey::text, ' ')::smallint[] as col_attnums
    from
        pg_catalog.pg_index pi
    where
        indisvalid
)
select
    'unindexed_foreign_keys' as name,
    'Unindexed foreign keys' as title,
    'INFO' as level,
    'EXTERNAL' as facing,
    array['PERFORMANCE'] as categories,
    'Identifies foreign key constraints without a covering index, which can impact database performance.' as description,
    format(
        'Table ''%s.%s'' has a foreign key ''%s'' without a covering index. This can lead to suboptimal query performance.',
        fk.schema_name,
        fk.table_name,
        fk.fkey_name
    ) as detail,
    'https://supabase.com/docs/guides/database/database-linter?lint=0001_unindexed_foreign_keys' as remediation,
    jsonb_build_object(
        'schema', fk.schema_name,
        'name', fk.table_name,
        'type', 'table',
        'fkey_name', fk.fkey_name,
        'fkey_columns', fk.col_attnums
    ) as metadata,
    format('unindexed_foreign_keys_%s_%s_%s', fk.schema_name, fk.table_name, fk.fkey_name) as cache_key
from
    foreign_keys fk
    left join index_ idx
        on fk.table_oid = idx.table_oid
        and fk.col_attnums = idx.col_attnums[1:array_length(fk.col_attnums, 1)]
    left join pg_catalog.pg_depend dep
        on idx.table_oid = dep.objid
        and dep.deptype = 'e'
where
    idx.index_ is null
    and fk.schema_name not in (
        '_timescaledb_cache', '_timescaledb_catalog', '_timescaledb_config', '_timescaledb_internal', 'auth', 'cron', 'extensions', 'graphql', 'graphql_public', 'information_schema', 'net', 'pgmq', 'pgroonga', 'pgsodium', 'pgsodium_masks', 'pgtle', 'pgbouncer', 'pg_catalog', 'pgtle', 'realtime', 'repack', 'storage', 'supabase_functions', 'supabase_migrations', 'tiger', 'topology', 'vault'
    )
    and dep.objid is null -- exclude tables owned by extensions
order by
    fk.schema_name,
    fk.table_name,
    fk.fkey_name)
union all
(
select
    'auth_users_exposed' as name,
    'Exposed Auth Users' as title,
    'ERROR' as level,
    'EXTERNAL' as facing,
    array['SECURITY'] as categories,
    'Detects if auth.users is exposed to anon or authenticated roles via a view or materialized view in schemas exposed to PostgREST, potentially compromising user data security.' as description,
    format(
        'View/Materialized View "%s" in the public schema may expose ''auth.users'' data to anon or authenticated roles.',
        c.relname
    ) as detail,
    'https://supabase.com/docs/guides/database/database-linter?lint=0002_auth_users_exposed' as remediation,
    jsonb_build_object(
        'schema', n.nspname,
        'name', c.relname,
        'type', 'view',
        'exposed_to', array_remove(array_agg(DISTINCT case when pg_catalog.has_table_privilege('anon', c.oid, 'SELECT') then 'anon' when pg_catalog.has_table_privilege('authenticated', c.oid, 'SELECT') then 'authenticated' end), null)
    ) as metadata,
    format('auth_users_exposed_%s_%s', n.nspname, c.relname) as cache_key
from
    -- Identify the oid for auth.users
    pg_catalog.pg_class auth_users_pg_class
    join pg_catalog.pg_namespace auth_users_pg_namespace
        on auth_users_pg_class.relnamespace = auth_users_pg_namespace.oid
        and auth_users_pg_class.relname = 'users'
        and auth_users_pg_namespace.nspname = 'auth'
    -- Depends on auth.users
    join pg_catalog.pg_depend d
        on d.refobjid = auth_users_pg_class.oid
    join pg_catalog.pg_rewrite r
        on r.oid = d.objid
    join pg_catalog.pg_class c
        on c.oid = r.ev_class
    join pg_catalog.pg_namespace n
        on n.oid = c.relnamespace
    join pg_catalog.pg_class pg_class_auth_users
        on d.refobjid = pg_class_auth_users.oid
where
    d.deptype = 'n'
    and (
      pg_catalog.has_table_privilege('anon', c.oid, 'SELECT')
      or pg_catalog.has_table_privilege('authenticated', c.oid, 'SELECT')
    )
    and n.nspname = any(array(select trim(unnest(string_to_array(current_setting('pgrst.db_schemas', 't'), ',')))))
    -- Exclude self
    and c.relname <> '0002_auth_users_exposed'
    -- There are 3 insecure configurations
    and
    (
        -- Materialized views don't support RLS so this is insecure by default
        (c.relkind in ('m')) -- m for materialized view
        or
        -- Standard View, accessible to anon or authenticated that is security_definer
        (
            c.relkind = 'v' -- v for view
            -- Exclude security invoker views
            and not (
                lower(coalesce(c.reloptions::text,'{}'))::text[]
                && array[
                    'security_invoker=1',
                    'security_invoker=true',
                    'security_invoker=yes',
                    'security_invoker=on'
                ]
            )
        )
        or
        -- Standard View, security invoker, but no RLS enabled on auth.users
        (
            c.relkind in ('v') -- v for view
            -- is security invoker
            and (
                lower(coalesce(c.reloptions::text,'{}'))::text[]
                && array[
                    'security_invoker=1',
                    'security_invoker=true',
                    'security_invoker=yes',
                    'security_invoker=on'
                ]
            )
            and not pg_class_auth_users.relrowsecurity
        )
    )
group by
    n.nspname,
    c.relname,
    c.oid)
`
