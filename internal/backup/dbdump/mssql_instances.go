package dbdump

import (
	"fmt"
	"sort"
	"strings"
)

// mssqlDefaultInstance is the name SQL Server registers its default
// (unnamed) instance under — the one that listens on port 1433.
const mssqlDefaultInstance = "MSSQLSERVER"

// localInstances lists the SQL Server instances installed on this machine.
// A variable so tests can substitute their own list.
var localInstances = installedMSSQLInstances

// withLocalInstance fills in the named instance for a connection that says
// nothing about where SQL Server is. Without it such a connection goes to
// localhost:1433, which only a default instance answers — and SQL Server
// Express installs as the named instance SQLEXPRESS, so out of the box the
// agent was refused on exactly the setup most small servers have.
//
// A connection that sets host, port or instance is left alone: someone
// decided where to connect, and that is not second-guessed.
func withLocalInstance(conn Conn) (Conn, error) {
	if conn.Host != "" || conn.Port != 0 || conn.Instance != "" {
		return conn, nil
	}
	instance, err := pickInstance(localInstances())
	if err != nil {
		return conn, err
	}
	conn.Instance = instance
	return conn, nil
}

// pickInstance chooses which of the installed instances to connect to when
// the configuration names none. It returns "" for the default instance.
func pickInstance(installed []string) (string, error) {
	var named []string
	for _, name := range installed {
		if strings.EqualFold(name, mssqlDefaultInstance) {
			// A default instance is what an unqualified connection has
			// always meant, so it wins over any named ones beside it.
			return "", nil
		}
		named = append(named, name)
	}
	switch len(named) {
	case 0:
		// Nothing found (or not Windows): keep the plain localhost
		// connection and let the driver report what it finds.
		return "", nil
	case 1:
		return named[0], nil
	}
	sort.Strings(named)
	return "", fmt.Errorf(
		"mssql: this machine has several SQL Server instances (%s) and none is the default — "+
			"set databases.mssql.default.instance in the agent's config file to choose one",
		strings.Join(named, ", "))
}
