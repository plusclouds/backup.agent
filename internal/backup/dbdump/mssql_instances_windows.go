package dbdump

import "golang.org/x/sys/windows/registry"

// installedMSSQLInstances reads the instance names SQL Server setup records
// in the registry. Both registry views are read, so a 32-bit SQL Server on
// 64-bit Windows is found too. Any error just means "none found": this is a
// convenience for an unconfigured connection, never a reason to fail.
func installedMSSQLInstances() []string {
	const path = `SOFTWARE\Microsoft\Microsoft SQL Server\Instance Names\SQL`

	seen := map[string]bool{}
	var names []string
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		values, err := key.ReadValueNames(-1)
		key.Close() //nolint:errcheck
		if err != nil {
			continue
		}
		for _, name := range values {
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
