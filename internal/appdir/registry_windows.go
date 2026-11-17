//go:build windows

package appdir

import "golang.org/x/sys/windows/registry"

func readRegistryPaths() RegistryPaths {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\EverGate`, registry.QUERY_VALUE)
	if err != nil {
		return RegistryPaths{}
	}
	defer key.Close()
	configPath, _, _ := key.GetStringValue("ConfigPath")
	dataDir, _, _ := key.GetStringValue("DataDir")
	return RegistryPaths{ConfigPath: configPath, DataDir: dataDir}
}
