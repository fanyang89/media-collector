package xhs

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/playwright-community/playwright-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

var GetLikesCmd = &cli.Command{
	Name:    "likes",
	Aliases: []string{"l"},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config-xhs.yml",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "./output",
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		outputPath := command.String("output")
		_, err := os.Stat(outputPath)
		if err != nil && os.IsNotExist(err) {
			err = os.Mkdir(outputPath, 0755)
			if err != nil {
				return err
			}
		}

		configPath := command.String("config")
		config, err := LoadConfig(configPath)
		if err != nil {
			return err
		}

		pw, err := playwright.Run()
		if err != nil {
			return err
		}

		browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(false),
		})
		if err != nil {
			return err
		}

		page, err := browser.NewPage()
		if err != nil {
			return err
		}

		err = page.AddInitScript(playwright.Script{Path: playwright.String("stealth.min.js")})
		if err != nil {
			return err
		}

		needLogin := true

		var token, source string
		if len(config.Cookies) > 0 {
			err = page.Context().AddCookies(config.Cookies)
			if err != nil {
				return err
			}

			token, source, err = getXsecToken(page)
			if err != nil {
				return err
			}

			client := newClient(page, config.GetCookies())
			_, err = client.GetMyInfo()
			_ = client.Close()

			if err != nil {
				var xerr *Error
				if errors.As(err, &xerr) {
					if xerr.Code == -100 {
						needLogin = true
					}
				} else {
					return err
				}
			}
		}

		if needLogin {
			cookies, err := login(page)
			if err != nil {
				return err
			}

			config.Cookies = cookies
			err = SaveConfig(configPath, config)
			if err != nil {
				return err
			}
		}

		client := newClient(page, config.GetCookies())
		client.SetXsecToken(token, source)
		defer func() { _ = client.Close() }()

		myInfo, err := client.GetMyInfo()
		if err != nil {
			return err
		}
		client.SetUserID(myInfo.UserID)

		_, err = page.Goto(fmt.Sprintf("https://www.xiaohongshu.com/user/profile/%s?tab=liked", myInfo.UserID))
		if err != nil {
			return err
		}

		// #userPageContainer > div.feeds-tab-container > div > div:nth-child(3) > div.feeds-container > section:nth-child(1)

		err = scrollPage(page)
		if err != nil {
			return err
		}

		err = browser.Close()
		if err != nil {
			return err
		}

		return pw.Stop()
	},
}

func scrollPage(page playwright.Page) error {
	for i := 0; i < 3; i++ {
		err := page.Keyboard().Press("PageDown")
		if err != nil {
			return err
		}
	}
	return nil
}

var BotTestPageCmd = &cli.Command{
	Name:    "bot-test",
	Aliases: []string{"bt"},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "proxy",
			Aliases: []string{"p"},
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		pw, err := playwright.Run()
		if err != nil {
			return err
		}

		options := playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(false),
		}
		proxy := command.String("proxy")
		if proxy != "" {
			options.Proxy = &playwright.Proxy{Server: proxy}
		}

		browser, err := pw.Chromium.Launch(options)
		if err != nil {
			return err
		}

		page, err := browser.NewPage()
		if err != nil {
			return err
		}

		err = page.AddInitScript(playwright.Script{Path: playwright.String("stealth.min.js")})
		if err != nil {
			return err
		}

		_, err = page.Goto("https://bot.sannysoft.com")
		if err != nil {
			return err
		}

		time.Sleep(time.Minute)

		err = browser.Close()
		if err != nil {
			return err
		}

		return pw.Stop()
	},
}

var RootCmd = &cli.Command{
	Name:    "xhs",
	Aliases: []string{"x"},
	Commands: []*cli.Command{
		GetLikesCmd,
		BotTestPageCmd,
	},
}

func getXsecToken(page playwright.Page) (token string, source string, err error) {
	_, err = page.Goto("https://www.xiaohongshu.com/explore")
	if err != nil {
		return
	}

	locator := page.Locator(`#exploreFeeds > section:nth-child(1) > div > a.cover.mask.ld`)
	val, err := locator.GetAttribute("href")
	if err != nil {
		return
	}

	u, err := url.Parse(val)
	if err != nil {
		return
	}

	token = u.Query().Get("xsec_token")
	source = u.Query().Get("xsec_source")
	return
}

type Signed struct {
	XS string `json:"X-s"`
	XT int    `json:"X-t"`
}

func sign(page playwright.Page, url string, data interface{}) (*Signed, error) {
	val, err := page.Evaluate(`(url, data) => window._webmsxyw(url, data)`, map[string]interface{}{
		"url":  url,
		"data": data,
	})
	if err != nil {
		return nil, err
	}

	s := val.(map[string]interface{})
	return &Signed{
		XS: s["X-s"].(string),
		XT: s["X-t"].(int),
	}, nil
}

func login(page playwright.Page) ([]playwright.OptionalCookie, error) {
	_, err := page.Goto("https://www.xiaohongshu.com/login")
	if err != nil {
		return nil, err
	}
	zap.L().Info("Waiting for login")

	err = page.WaitForURL("https://www.xiaohongshu.com/explore")
	if err != nil {
		return nil, err
	}

	cookies, err := page.Context().Cookies()
	if err != nil {
		return nil, err
	}

	zap.L().Info("Login success")
	return convertToOptionalCookieSlice(cookies), nil
}

func convertToOptionalCookieSlice(cookies []playwright.Cookie) []playwright.OptionalCookie {
	cs := make([]playwright.OptionalCookie, len(cookies))
	for i, cookie := range cookies {
		c := playwright.OptionalCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Secure:   nil,
			SameSite: nil,
		}
		if cookie.Domain != "" {
			d := cookie.Domain
			c.Domain = &d
		}
		if cookie.Path != "" {
			p := cookie.Path
			c.Path = &p
		}
		if cookie.Expires != 0 {
			v := cookie.Expires
			c.Expires = &v
		}
		if cookie.HttpOnly {
			v := cookie.HttpOnly
			c.HttpOnly = &v
		}
		if cookie.Secure {
			v := cookie.Secure
			c.Secure = &v
		}
		if cookie.SameSite != nil {
			v := *cookie.SameSite
			c.SameSite = &v
		}
		cs[i] = c
	}
	return cs
}
