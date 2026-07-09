package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type ProjectConfig struct {
	Name      string
	Languages map[string]string
}

type tomlConfig struct {
	Project struct {
		Name string `mapstructure:"name"`
	} `mapstructure:"project"`
	Languages map[string]struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"languages"`
}

// LoadConfig localiza o arquivo heddle.toml subindo diretórios e o lê usando Viper.
// Se heddle.toml não for encontrado, retorna uma configuração padrão.
func LoadConfig() (*ProjectConfig, error) {
	root, err := FindProjectRoot()
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(root, "heddle.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		return &ProjectConfig{
			Name:      "default-project",
			Languages: make(map[string]string),
		}, nil
	}
	return LoadConfigFromFile(tomlPath)
}

// LoadConfigFromFile carrega a configuração de um caminho absoluto específico (útil para testes).
func LoadConfigFromFile(path string) (*ProjectConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var raw tomlConfig
	if err := v.Unmarshal(&raw); err != nil {
		return nil, err
	}

	cfg := &ProjectConfig{
		Name:      raw.Project.Name,
		Languages: make(map[string]string),
	}

	for lang, langCfg := range raw.Languages {
		cfg.Languages[lang] = langCfg.Path
	}

	return cfg, nil
}

// FindProjectRoot localiza a pasta raiz contendo o heddle.toml.
// Se não encontrar, tenta encontrar go.mod ou a pasta pkg/lib subindo os diretórios.
// Em última análise, retorna a pasta de execução atual.
func FindProjectRoot() (string, error) {
	startDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "heddle.toml")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "pkg", "lib")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return startDir, nil
}

// ResolveLocalPackagePath resolve o caminho físico absoluto de um pacote mapeado no toml.
func (c *ProjectConfig) ResolveLocalPackagePath(projectRoot, lang, pkgPath string) (string, bool) {
	relPath, exists := c.Languages[lang]
	if !exists {
		if lang == "golang" {
			return filepath.Join(projectRoot, "pkg/lib", pkgPath), true
		}
		return "", false
	}
	return filepath.Join(projectRoot, relPath, pkgPath), true
}

// FindProjectRootFrom localiza a pasta raiz contendo o heddle.toml a partir de um diretório inicial.
func FindProjectRootFrom(startDir string) (string, error) {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "heddle.toml")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "pkg", "lib")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir, nil
}

// LoadConfigFromDir carrega a configuração a partir de um diretório inicial.
func LoadConfigFromDir(dir string) (*ProjectConfig, error) {
	root, err := FindProjectRootFrom(dir)
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(root, "heddle.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		return &ProjectConfig{
			Name:      "default-project",
			Languages: make(map[string]string),
		}, nil
	}
	return LoadConfigFromFile(tomlPath)
}
