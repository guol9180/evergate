// Package appdir 解析 EverGate 的配置与数据目录。
package appdir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

const (
	SourceExplicit    = "explicit"
	SourceEnvironment = "environment"
	SourceRegistry    = "registry"
	SourceDefault     = "default"
	SourcePortable    = "portable"
	SourceWorkingDir  = "working-directory"
)

// RegistryPaths 是安装器写入 HKCU\Software\EverGate 的路径。
type RegistryPaths struct {
	ConfigPath string
	DataDir    string
}

// Options 是路径发现所需的运行环境；测试可完整注入。
type Options struct {
	ExplicitConfig string
	Explicit       bool
	GOOS           string
	WorkingDir     string
	Executable     string
	HomeDir        string
	LocalAppData   string
	Environment    func(string) string
	Registry       RegistryPaths
}

// Paths 是最终选定的配置位置与默认数据目录。
type Paths struct {
	ConfigPath   string
	ConfigDir    string
	DataDir      string
	Source       string
	ConfigExists bool
}

// Discover 读取当前进程环境、可执行文件位置和 Windows 注册表。
func Discover(explicitConfig string, explicit bool) (Paths, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return Paths{}, err
	}
	executable, _ := os.Executable()
	homeDir, _ := os.UserHomeDir()
	return Resolve(Options{
		ExplicitConfig: explicitConfig,
		Explicit:       explicit,
		GOOS:           runtime.GOOS,
		WorkingDir:     workingDir,
		Executable:     executable,
		HomeDir:        homeDir,
		LocalAppData:   os.Getenv("LOCALAPPDATA"),
		Environment:    os.Getenv,
		Registry:       readRegistryPaths(),
	})
}

// Resolve 按显式参数 → EVERGATE_CONFIG → 注册表 → 平台默认 → 便携目录 → cwd 选择配置。
func Resolve(opts Options) (Paths, error) {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.WorkingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Paths{}, err
		}
		opts.WorkingDir = cwd
	}
	if opts.HomeDir == "" {
		opts.HomeDir, _ = os.UserHomeDir()
	}
	getenv := opts.Environment
	if getenv == nil {
		getenv = os.Getenv
	}
	if opts.LocalAppData == "" {
		opts.LocalAppData = getenv("LOCALAPPDATA")
	}

	defaultConfig, defaultData, err := platformDefaults(opts, getenv)
	if err != nil {
		return Paths{}, err
	}
	registryDataDir := defaultData
	if opts.Registry.DataDir != "" {
		registryDataDir = absoluteFrom(opts.Registry.DataDir, opts.WorkingDir)
	}

	if opts.Explicit && opts.ExplicitConfig != "" {
		return selectedPaths(opts.ExplicitConfig, opts.WorkingDir, defaultData, SourceExplicit)
	}
	if envConfig := getenv("EVERGATE_CONFIG"); envConfig != "" {
		return selectedPaths(envConfig, opts.WorkingDir, defaultData, SourceEnvironment)
	}
	if opts.Registry.ConfigPath != "" {
		return selectedPaths(opts.Registry.ConfigPath, opts.WorkingDir, registryDataDir, SourceRegistry)
	}

	if exists, err := fileExists(defaultConfig); err != nil {
		return Paths{}, err
	} else if exists {
		return pathsFor(defaultConfig, defaultData, SourceDefault, true), nil
	}

	if opts.Executable != "" {
		portable := filepath.Join(filepath.Dir(opts.Executable), "evergate.yml")
		if exists, err := fileExists(portable); err != nil {
			return Paths{}, err
		} else if exists {
			return pathsFor(portable, defaultData, SourcePortable, true), nil
		}
	}

	cwdConfig := filepath.Join(opts.WorkingDir, "evergate.yml")
	if exists, err := fileExists(cwdConfig); err != nil {
		return Paths{}, err
	} else if exists {
		return pathsFor(cwdConfig, defaultData, SourceWorkingDir, true), nil
	}

	return pathsFor(defaultConfig, defaultData, SourceDefault, false), nil
}

// SQLitePath 解析配置中的 SQLite 路径；空值使用数据目录默认文件。
func SQLitePath(configPath, dataDir, configured string) (string, error) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	switch {
	case configured == "":
		if dataDir == "" {
			return "", errors.New("default data directory is empty")
		}
		return filepath.Join(dataDir, "evergate.db"), nil
	case filepath.IsAbs(configured):
		return filepath.Clean(configured), nil
	default:
		return filepath.Join(filepath.Dir(configPath), configured), nil
	}
}

func platformDefaults(opts Options, getenv func(string) string) (string, string, error) {
	switch opts.GOOS {
	case "windows":
		localAppData := opts.LocalAppData
		if localAppData == "" {
			localAppData = filepath.Join(opts.HomeDir, "AppData", "Local")
		}
		root := filepath.Join(localAppData, "EverGate")
		return filepath.Join(root, "evergate.yml"), filepath.Join(root, "data"), nil
	case "darwin":
		if opts.HomeDir == "" {
			return "", "", errors.New("home directory is required for macOS defaults")
		}
		root := filepath.Join(opts.HomeDir, "Library", "Application Support", "EverGate")
		return filepath.Join(root, "evergate.yml"), filepath.Join(root, "data"), nil
	default:
		configHome := getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			if opts.HomeDir == "" {
				return "", "", errors.New("home directory is required for Linux defaults")
			}
			configHome = filepath.Join(opts.HomeDir, ".config")
		}
		dataHome := getenv("XDG_DATA_HOME")
		if dataHome == "" {
			if opts.HomeDir == "" {
				return "", "", errors.New("home directory is required for Linux defaults")
			}
			dataHome = filepath.Join(opts.HomeDir, ".local", "share")
		}
		return filepath.Join(configHome, "evergate", "evergate.yml"),
			filepath.Join(dataHome, "evergate"), nil
	}
}

func selectedPaths(configPath, workingDir, dataDir, source string) (Paths, error) {
	absConfig := absoluteFrom(configPath, workingDir)
	exists, err := fileExists(absConfig)
	if err != nil {
		return Paths{}, err
	}
	return pathsFor(absConfig, dataDir, source, exists), nil
}

func pathsFor(configPath, dataDir, source string, exists bool) Paths {
	return Paths{
		ConfigPath:   configPath,
		ConfigDir:    filepath.Dir(configPath),
		DataDir:      filepath.Clean(dataDir),
		Source:       source,
		ConfigExists: exists,
	}
}

func absoluteFrom(path, base string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
