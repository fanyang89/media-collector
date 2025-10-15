package bilibili

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flytam/filenamify"
	"github.com/go-resty/resty/v2"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/CuteReimu/bilibili/v2"
)

const readStreamSliceTimeout = 30 * time.Second

type VideoAudioPair struct {
	VideoPath  string
	AudioPath  string
	OutputPath string
}

func defaultExecutableFileExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

var loginCmd = &cli.Command{
	Name:  "login",
	Usage: "Login and save cookies",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yml",
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		configPath := command.String("config")
		config, err := LoadConfig(configPath)
		if err != nil {
			return err
		}

		client := bilibili.New()
		cookies, err := Login(client)
		if err != nil {
			return err
		}

		config.Cookies = cookies
		return SaveConfig(configPath, config)
	},
}

var downloadCmd = &cli.Command{
	Name:  "download",
	Usage: "Download video",
	Commands: []*cli.Command{
		downloadToViewCmd,
		downloadSingleCmd,
		downloadSearchCmd,
	},
}

func convertAidToBvid(aid int) string {
	const base58CharTable = "fZodR9XQDSUm21yCkr6zBqiveYah8bt4xsWpHnJE7jL5VG3guMTKNPAwcF"
	const xorN = 177451812
	const addN = 8728348608
	s := []int{11, 10, 3, 8, 4, 6}
	z := (aid ^ xorN) + addN
	l := []rune("BV1  4 1 7  ")
	for i, c := range s {
		n := int(math.Floor(float64(z)/(math.Pow(58, float64(i))))) % 58
		l[c] = rune(base58CharTable[n])
	}
	return string(l)
}

func NewGetVideoStreamParam(bvid string, cid int) bilibili.GetVideoStreamParam {
	return bilibili.GetVideoStreamParam{
		Bvid:     bvid,
		Cid:      cid,
		Platform: "pc",
		// https://socialsisteryi.github.io/bilibili-API-collect/docs/video/videostream_url.html#fnval%E8%A7%86%E9%A2%91%E6%B5%81%E6%A0%BC%E5%BC%8F%E6%A0%87%E8%AF%86
		Fnval: 16 | 128,
	}
}

var downloadToViewCmd = &cli.Command{
	Name:  "to-view",
	Usage: "Download to-view (playback later) videos",
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
		&cli.StringFlag{
			Name:  "ffmpeg",
			Value: "ffmpeg" + defaultExecutableFileExtension(),
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		d, err := downloaderFromCliCommand(command)
		if err != nil {
			return err
		}

		toViewList, err := d.GetClient().GetToViewList()
		if err != nil {
			return err
		}

		for _, v := range toViewList.List {
			err = d.Download(DownloadOption{
				Bvid:      v.Bvid,
				Cid:       v.Cid,
				OwnerName: v.Owner.Name,
				Title:     v.Title,
			}, false, true)
			if err != nil {
				zap.L().Error("Download failed", zap.String("bvid", v.Bvid), zap.Error(err))
				continue
			}
		}

		return nil
	},
}

var RootCmd = &cli.Command{
	Name:    "bilibili",
	Usage:   "Commands for Bilibili",
	Aliases: []string{"b"},
	Commands: []*cli.Command{
		loginCmd,
		downloadCmd,
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

func copyRestyClient(c *resty.Client) *resty.Client {
	cc := *c
	return &cc
}

var ErrFileTooLarge = errors.New("file too large")

func (d *Downloader) downloadSingleFile(filePath string, url string) error {
	fileName := filepath.Base(filePath)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	client := d.GetClient()
	c := copyRestyClient(client.Resty())
	c.SetTimeout(20 * time.Minute)

	rsp, err := c.R().SetDoNotParseResponse(true).Get(url)
	if err != nil {
		return err
	}
	body := rsp.RawBody()
	defer func() { _ = body.Close() }()

	fmt.Printf("Downloading %s\n", fileName)
	contentLength := getContentLength(rsp.Header())
	if d.maxFileSize > 0 && contentLength >= d.maxFileSize {
		return errors.Wrapf(ErrFileTooLarge, "file: %s", fileName)
	}

	bar := NewProgressBar(contentLength, "")
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

func (d *Downloader) DownloadFile(filePath string, urls []string) error {
	if len(urls) == 0 {
		return errors.New("urls is empty")
	}

	if len(urls) > 1 {
		for _, url := range urls {
			err := d.downloadSingleFile(filePath, url)
			if err != nil {
				if errors.Is(err, ErrFileTooLarge) {
					return err
				}
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
			err := d.downloadSingleFile(filePath, urls[0])
			if err != nil {
				if errors.Is(err, ErrFileTooLarge) {
					return err
				}
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

func newFileName(author string, title string, suffix string, format string) string {
	if strings.Contains(format, "mp4") {
		format = "mp4"
	} else if strings.Contains(format, "flv") {
		format = "flv"
	}
	if suffix != "" {
		suffix = "_" + suffix
	}

	fileName := fmt.Sprintf("%s - %s%s.%s", author, title, suffix, format)
	fileName, err := filenamify.FilenamifyV2(fileName)
	if err != nil {
		panic(err)
	}
	return fileName
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
