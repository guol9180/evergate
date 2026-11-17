package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDiscoveryPrecedence(t *testing.T) {
	root := t.TempDir()
	workingDir := filepath.Join(root, "cwd")
	exeDir := filepath.Join(root, "portable")
	configHome := filepath.Join(root, "config-home")
	dataHome := filepath.Join(root, "data-home")
	for _, dir := range []string{workingDir, exeDir, configHome, dataHome} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(workingDir, "evergate.yml"))
	writeFile(t, filepath.Join(exeDir, "evergate.yml"))
	defaultConfig := filepath.Join(configHome, "evergate", "evergate.yml")
	if err := os.MkdirAll(filepath.Dir(defaultConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, defaultConfig)

	base := Options{
		GOOS:       "linux",
		WorkingDir: workingDir,
		Executable: filepath.Join(exeDir, "evergate"),
		HomeDir:    root,
		Environment: func(key string) string {
			switch key {
			case "XDG_CONFIG_HOME":
				return configHome
			case "XDG_DATA_HOME":
				return dataHome
			default:
				return ""
			}
		},
	}
	got, err := Resolve(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != defaultConfig || got.Source != SourceDefault || !got.ConfigExists {
		t.Fatalf("default config should win after explicit/env/registry: %#v", got)
	}

	if err := os.Remove(defaultConfig); err != nil {
		t.Fatal(err)
	}
	got, err = Resolve(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != filepath.Join(exeDir, "evergate.yml") || got.Source != SourcePortable {
		t.Fatalf("portable config should win over cwd: %#v", got)
	}

	if err := os.Remove(filepath.Join(exeDir, "evergate.yml")); err != nil {
		t.Fatal(err)
	}
	got, err = Resolve(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != filepath.Join(workingDir, "evergate.yml") || got.Source != SourceWorkingDir {
		t.Fatalf("cwd config should be last existing fallback: %#v", got)
	}
}

func TestResolveExplicitEnvAndRegistry(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "explicit.yml")
	envPath := filepath.Join(root, "env.yml")
	registryPath := filepath.Join(root, "registry.yml")
	registryData := filepath.Join(root, "registry-data")
	opts := Options{
		ExplicitConfig: explicit,
		Explicit:       true,
		GOOS:           "windows",
		WorkingDir:     root,
		Executable:     filepath.Join(root, "EverGate.exe"),
		HomeDir:        root,
		LocalAppData:   filepath.Join(root, "local"),
		Environment: func(key string) string {
			if key == "EVERGATE_CONFIG" {
				return envPath
			}
			return ""
		},
		Registry: RegistryPaths{ConfigPath: registryPath, DataDir: registryData},
	}
	got, err := Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != explicit || got.Source != SourceExplicit || got.DataDir != filepath.Join(root, "local", "EverGate", "data") {
		t.Fatalf("explicit config should win: %#v", got)
	}

	opts.Explicit = false
	got, err = Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != envPath || got.Source != SourceEnvironment {
		t.Fatalf("env config should win without explicit flag: %#v", got)
	}

	opts.Environment = func(string) string { return "" }
	got, err = Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != registryPath || got.Source != SourceRegistry || got.DataDir != registryData {
		t.Fatalf("registry config should win: %#v", got)
	}
}

func TestResolvePlatformDefaultsAndMissingConfig(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name       string
		goos       string
		env        map[string]string
		wantConfig string
		wantData   string
	}{
		{
			name: "windows", goos: "windows",
			env:        map[string]string{"LOCALAPPDATA": filepath.Join(root, "Local")},
			wantConfig: filepath.Join(root, "Local", "EverGate", "evergate.yml"),
			wantData:   filepath.Join(root, "Local", "EverGate", "data"),
		},
		{
			name: "linux", goos: "linux",
			env: map[string]string{
				"XDG_CONFIG_HOME": filepath.Join(root, "config"),
				"XDG_DATA_HOME":   filepath.Join(root, "data"),
			},
			wantConfig: filepath.Join(root, "config", "evergate", "evergate.yml"),
			wantData:   filepath.Join(root, "data", "evergate"),
		},
		{
			name: "darwin", goos: "darwin",
			wantConfig: filepath.Join(root, "Library", "Application Support", "EverGate", "evergate.yml"),
			wantData:   filepath.Join(root, "Library", "Application Support", "EverGate", "data"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{
				GOOS: tc.goos, WorkingDir: filepath.Join(root, tc.name, "cwd"),
				Executable: filepath.Join(root, tc.name, "evergate"),
				HomeDir:    root, LocalAppData: tc.env["LOCALAPPDATA"],
				Environment: func(key string) string { return tc.env[key] },
			}
			if err := os.MkdirAll(opts.WorkingDir, 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := Resolve(opts)
			if err != nil {
				t.Fatal(err)
			}
			if got.ConfigPath != tc.wantConfig || got.DataDir != tc.wantData || got.ConfigExists || got.Source != SourceDefault {
				t.Fatalf("paths = %#v", got)
			}
		})
	}
}

func TestSQLitePathResolution(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "evergate.yml")
	defaultData := filepath.Join(root, "data")

	got, err := SQLitePath(configPath, defaultData, "./db/evergate.db")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "config", "db", "evergate.db"); got != want {
		t.Fatalf("relative sqlite path = %q, want %q", got, want)
	}

	got, err = SQLitePath(configPath, defaultData, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(defaultData, "evergate.db"); got != want {
		t.Fatalf("empty sqlite path = %q, want %q", got, want)
	}

	absolute := filepath.Join(root, "custom", "evergate.db")
	got, err = SQLitePath(configPath, defaultData, absolute)
	if err != nil {
		t.Fatal(err)
	}
	if got != absolute {
		t.Fatalf("absolute sqlite path = %q, want %q", got, absolute)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
