package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed default.toml
var defaultTemplate []byte

// FileStore 把配置解析与文件权限、原子替换机制分开。
type FileStore interface {
	Path() (string, error)
	ReadFile(string) ([]byte, error)
	WritePrivateFile(string, []byte) error
	EnsurePrivateFile(string, []byte) error
}

// Store 每次读取文件和环境变量，调用方在一次执行内复用返回的 Config。
type Store struct{ Files FileStore }

// Path 返回配置文件路径。
func (s Store) Path() (string, error) { return s.Files.Path() }

// EnsureDefaultConfigFile 在文件缺失时原样写出带注释的模板。
func (s Store) EnsureDefaultConfigFile() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	return s.Files.EnsurePrivateFile(path, defaultTemplate)
}

func (s Store) readFile() ([]byte, error) {
	path, err := s.Path()
	if err != nil {
		return nil, err
	}
	body, err := s.Files.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return body, err
}

// Current 先应用默认值，再解码文件并按存在性覆盖环境变量；未知文件键被忽略。
func (s Store) Current() (Config, error) {
	c := Default()
	if err := s.load(&c); err != nil {
		return Config{}, err
	}
	if err := validate(&c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (s Store) load(c *Config) error {
	body, err := s.readFile()
	if err != nil {
		return err
	}
	if err := toml.Unmarshal(body, c); err != nil {
		// go-toml 的消息只带 Go 字段名，还可能回显配置值（例如溢出的数字）；
		// 这里只报告行号与该行的键，不转述原始消息。
		var decodeErr *toml.DecodeError
		if !errors.As(err, &decodeErr) {
			return errors.New("config.toml: invalid TOML")
		}
		line, _ := decodeErr.Position()
		lines := strings.Split(string(body), "\n")
		name, _, _ := strings.Cut(lines[min(max(line, 1), len(lines))-1], "=")
		name = strings.TrimSpace(name)
		// 出错行可能位于多行字符串内，等号左侧是配置值；只报告已声明的键名。
		if slices.ContainsFunc(Keys(), func(key string) bool { return key == name || strings.HasSuffix(key, "."+name) }) {
			return fmt.Errorf("config.toml line %d: key %q has invalid TOML or a value of the wrong type", line, name)
		}
		return fmt.Errorf("config.toml line %d: invalid TOML", line)
	}
	applyEnvironment(c)
	return nil
}
