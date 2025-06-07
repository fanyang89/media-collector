package bilibili

import (
	"os"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Cookies   string `yaml:"cookies"`
	Output    string `yaml:"output"`
	FFmpeg    string `yaml:"ffmpeg"`
	HistoryDB string `yaml:"history_db"`
}

func defaultConfig() *Config {
	return &Config{
		Cookies:   "",
		Output:    "./output",
		FFmpeg:    "ffmpeg" + defaultExecutableFileExtension(),
		HistoryDB: "./media-collector.db",
	}
}

func LoadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(buf, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func SaveConfig(path string, config *Config) error {
	buf, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0644)
}
