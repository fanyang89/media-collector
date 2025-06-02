package xhs

import (
	"net/http"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/playwright-community/playwright-go"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Cookies []playwright.OptionalCookie `yaml:"cookies"`
}

func (c *Config) GetCookies() []*http.Cookie {
	cookies := make([]*http.Cookie, len(c.Cookies))
	for i, cookie := range c.Cookies {
		x := http.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		}
		if cookie.Domain != nil {
			x.Domain = *cookie.Domain
		}
		if cookie.Path != nil {
			x.Path = *cookie.Path
		}
		if cookie.Expires != nil {
			x.Expires = time.Unix(int64(*cookie.Expires), 0)
		}
		if cookie.HttpOnly != nil {
			x.HttpOnly = *cookie.HttpOnly
		}
		if cookie.Secure != nil {
			x.Secure = *cookie.Secure
		}
		if cookie.SameSite != nil {
			switch *cookie.SameSite {
			case "Strict":
				x.SameSite = http.SameSiteStrictMode
			case "None":
				x.SameSite = http.SameSiteNoneMode
			case "Lax":
				fallthrough
			default:
				x.SameSite = http.SameSiteLaxMode
			}
		}
		cookies[i] = &x
	}
	return cookies
}

func LoadConfig(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
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
