//go:build !windows

package appdir

func readRegistryPaths() RegistryPaths {
	return RegistryPaths{}
}
