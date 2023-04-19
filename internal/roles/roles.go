package roles

import (
	"github.com/dalibo/ldap2pg/internal/config"
	"github.com/dalibo/ldap2pg/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/lithammer/dedent"
)

type Role struct {
	Name    string
	Comment string
	Parents []string
	Options config.RoleOptions
}

type RoleSet map[string]Role

func NewRoleFromRow(row pgx.CollectableRow, instanceRoleColumns []string) (role Role, err error) {
	var variableRow interface{}
	err = row.Scan(&role.Name, &variableRow, &role.Comment, &role.Parents)
	if err != nil {
		return
	}
	record := variableRow.([]interface{})
	var colname string
	for i, value := range record {
		colname = instanceRoleColumns[i]
		switch colname {
		case "rolbypassrls":
			role.Options.ByPassRLS = value.(bool)
		case "rolcanlogin":
			role.Options.CanLogin = value.(bool)
		case "rolconnlimit":
			role.Options.ConnLimit = int(value.(int32))
		case "rolcreatedb":
			role.Options.CreateDB = value.(bool)
		case "rolcreaterole":
			role.Options.CreateRole = value.(bool)
		case "rolinherit":
			role.Options.Inherit = value.(bool)
		case "rolreplication":
			role.Options.Replication = value.(bool)
		case "rolsuper":
			role.Options.Super = value.(bool)
		}
	}
	return
}

func (r *Role) String() string {
	return r.Name
}

func (r *Role) BlacklistKey() string {
	return r.Name
}

// Generate queries to update current role configuration to match wanted role
// configuration.
func (r *Role) Alter(wanted Role, ch chan postgres.SyncQuery) {
	identifier := pgx.Identifier{r.Name}.Sanitize()

	if wanted.Options != r.Options {
		ch <- postgres.SyncQuery{
			Description: "Alter options.",
			LogArgs: []interface{}{
				"role", r.Name,
				"current", r.Options,
				"wanted", wanted.Options,
			},
			Query: `ALTER ROLE ` + identifier + ` WITH ` + wanted.Options.String() + `;`,
		}
	}
}

func (r *Role) Create(ch chan postgres.SyncQuery) {
	identifier := pgx.Identifier{r.Name}.Sanitize()

	ch <- postgres.SyncQuery{
		Description: "Create role.",
		LogArgs: []interface{}{
			"role", r.Name,
		},
		Query: `CREATE ROLE ` + identifier + ` ` + r.Options.String() + `;`,
	}
	ch <- postgres.SyncQuery{
		Description: "Set role comment.",
		LogArgs: []interface{}{
			"role", r.Name,
		},
		Query:     `COMMENT ON ROLE ` + identifier + ` IS $1;`,
		QueryArgs: []interface{}{r.Comment},
	}
}

func (r *Role) Drop(databases []string, ch chan postgres.SyncQuery) {
	identifier := pgx.Identifier{r.Name}.Sanitize()
	ch <- postgres.SyncQuery{
		Description: "Terminate running sessions.",
		LogArgs: []interface{}{
			"role", r.Name,
		},
		Query: dedent.Dedent(`
		SELECT pg_terminate_backend(pid)
		FROM pg_catalog.pg_stat_activity
		WHERE usename = $1;
		`),
		QueryArgs: []interface{}{r.Name},
	}
	for _, database := range databases {
		ch <- postgres.SyncQuery{
			Description: "Reassign objects and purge ACL.",
			LogArgs:     []interface{}{"role", r.Name, "database", database},
			Database:    database,
			Query: dedent.Dedent(`
			REASSIGN OWNED BY ` + identifier + ` TO CURRENT_USER;
			DROP OWNED BY ` + identifier + `;`),
		}
	}
	ch <- postgres.SyncQuery{
		Description: "Drop role.",
		LogArgs: []interface{}{
			"role", r.Name,
		},
		Query: `DROP ROLE ` + identifier + `;`,
	}
}