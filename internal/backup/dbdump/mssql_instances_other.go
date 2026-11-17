//go:build !windows

package dbdump

// installedMSSQLInstances has nothing to look at off Windows: SQL Server on
// Linux has no named instances, only the default one on its configured port.
func installedMSSQLInstances() []string { return nil }
