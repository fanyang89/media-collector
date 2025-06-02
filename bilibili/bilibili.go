package bilibili

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CuteReimu/bilibili/v2"
	"github.com/cockroachdb/errors"
	"github.com/flytam/filenamify"
	"github.com/go-resty/resty/v2"
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

const readStreamSliceTimeout = 30 * time.Second

var RootCmd = &cli.Command{
	Name:    "bilibili",
	Aliases: []string{"bili", "b"},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yml",
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

		client := bilibili.New()

		if config.Cookies != "" {
			client.SetCookiesString(config.Cookies)
		} else {
			c, err := Login(client)
			if err != nil {
				return err
			}

			config.Cookies = c
			err = SaveConfig(configPath, config)
			if err != nil {
				return err
			}
		}

		toViewList, err := client.GetToViewList()
		if err != nil {
			return err
		}

		for _, v := range toViewList.List {
			cid := v.Cid
			result, err := client.GetVideoStream(bilibili.GetVideoStreamParam{
				Bvid:        v.Bvid,
				Cid:         cid,
				Platform:    "html5",
				HighQuality: 1,
			})
			if err != nil {
				return err
			}
			for _, durl := range result.Durl {
				fileName := newName(v.Owner.Name, v.Title, durl.Order, result.Format)
				err = downloadFile(client, filepath.Join(outputPath, fileName), append([]string{durl.Url}, durl.BackupUrl...))
				if err != nil {
					return err
				}
			}
		}

		return nil
	},
}

func getContentLength(header http.Header) int64 {
	s := header.Get("Content-Length")
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	return v
}

func newProgressBar(maxBytes int64, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		maxBytes,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(15),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			_, _ = fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}

func copyRestyClient(c *resty.Client) *resty.Client {
	cc := *c
	return &cc
}

func downloadSingleFile(client *bilibili.Client, filePath string, url string) error {
	fileName := filepath.Base(filePath)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	c := copyRestyClient(client.Resty())
	c.SetTimeout(24 * time.Hour)

	rsp, err := c.R().SetDoNotParseResponse(true).Get(url)
	if err != nil {
		return err
	}
	body := rsp.RawBody()
	defer func() { _ = body.Close() }()

	fmt.Printf("Downloading %s\n", fileName)
	contentLength := getContentLength(rsp.Header())
	bar := newProgressBar(contentLength, "Downloading")
	defer func() { _ = bar.Finish() }()

	buf := make([]byte, 1*1024*1024)
	writer := io.MultiWriter(f, bar)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), readStreamSliceTimeout)
		var n int
		n, err = readWithContext(ctx, body, buf)
		cancel()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		_, err = writer.Write(buf[:n])
		if err != nil {
			return err
		}
	}
}

func readWithContext(ctx context.Context, r io.Reader, buf []byte) (n int, err error) {
	done := make(chan struct{})
	go func() {
		n, err = r.Read(buf)
		close(done)
	}()

	select {
	case <-done:
		return n, err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func downloadFile(client *bilibili.Client, filePath string, urls []string) error {
	_, err := os.Stat(filePath)
	if err == nil {
		return nil
	}

	if len(urls) == 0 {
		return errors.New("urls is empty")
	}

	if len(urls) > 1 {
		for _, url := range urls {
			err = downloadSingleFile(client, filePath, url)
			if err != nil {
				zap.L().Error("Download file failed, try next URL", zap.Error(err))
				continue
			}
			return nil
		}
	}

	if len(urls) == 1 {
		tryCnt := 0
		const maxTryCnt = 5
		const tryInterval = time.Second
		for tryCnt < maxTryCnt {
			tryCnt++
			err = downloadSingleFile(client, filePath, urls[0])
			if err != nil {
				zap.L().Error("Download file failed, try again later", zap.Error(err))
				time.Sleep(tryInterval)
			} else {
				return nil
			}
		}
	}

	fileName := filepath.Base(filePath)
	return errors.Newf("download %s failed", fileName)
}

func newName(author string, title string, order int, format string) string {
	if strings.HasPrefix(format, "mp4") {
		format = "mp4"
	}

	fileName := fmt.Sprintf("%s - %s_%d.%s", author, title, order, format)
	fileName, err := filenamify.Filenamify(fileName, filenamify.Options{})
	if err != nil {
		panic(err)
	}
	return fileName
}

type Config struct {
	Cookies string `yaml:"cookies"`
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

func Login(client *bilibili.Client) (string, error) {
	qrCode, err := client.GetQRCode()
	if err != nil {
		return "", err
	}
	qrCode.Print()

	result, err := client.LoginWithQRCode(bilibili.LoginWithQRCodeParam{QrcodeKey: qrCode.QrcodeKey})
	if err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", errors.Newf("login failed: %s", result.Message)
	}

	zap.L().Info("Login success")
	return client.GetCookiesString(), nil
}
