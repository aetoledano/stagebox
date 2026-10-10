//go:build windows

package games

import "golang.org/x/sys/windows/registry"

func steamRegistryPaths() []string {
	paths := []string{}
	for _, source := range []struct {
		root        registry.Key
		path, value string
		view        uint32
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamExe", 0},
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath", 0},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath", registry.WOW64_32KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath", registry.WOW64_64KEY},
	} {
		key, err := registry.OpenKey(source.root, source.path, registry.QUERY_VALUE|source.view)
		if err != nil {
			continue
		}
		path, _, err := key.GetStringValue(source.value)
		_ = key.Close()
		if err == nil && path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
