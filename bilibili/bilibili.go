package bilibili

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CuteReimu/bilibili/v2"
	"github.com/cockroachdb/errors"
	"github.com/flytam/filenamify"
	"github.com/go-resty/resty/v2"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
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
	Name: "login",
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
	Name: "download",
	Commands: []*cli.Command{
		downloadToViewCmd,
	},
}

var downloadToViewCmd = &cli.Command{
	Name: "to-view",
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
		ffmpegPath := command.String("ffmpeg")
		_, err := os.Stat(ffmpegPath)
		if err != nil {
			return errors.Wrap(err, "ffmpeg not exist, please install ffmpeg first")
		}

		outputPath := command.String("output")
		_, err = os.Stat(outputPath)
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

		mergeList := make([]VideoAudioPair, 0)

		for _, v := range toViewList.List {
			cid := v.Cid
			result, err := client.GetVideoStream(bilibili.GetVideoStreamParam{
				Bvid:     v.Bvid,
				Cid:      cid,
				Platform: "pc",
				Fnval:    16,
			})
			if err != nil {
				return err
			}

			if len(result.Dash.Video) == 0 || len(result.Dash.Audio) == 0 {
				zap.L().Error("Can't get video stream",
					zap.Int("cid", cid), zap.String("title", v.Title), zap.String("owner", v.Owner.Name))
				continue
			}

			slices.SortFunc(result.Dash.Video, func(a, b bilibili.AudioOrVideo) int {
				return b.Bandwidth - a.Bandwidth
			})
			slices.SortFunc(result.Dash.Audio, func(a, b bilibili.AudioOrVideo) int {
				return b.Bandwidth - a.Bandwidth
			})

			video := result.Dash.Video[0]
			videoPath := filepath.Join(outputPath, newFileName(v.Owner.Name, v.Title, "video", video.MimeType))
			err = downloadFile(client, videoPath, append([]string{video.BaseUrl}, video.BackupUrl...))
			if err != nil {
				return err
			}

			audio := result.Dash.Audio[0]
			audioPath := filepath.Join(outputPath, newFileName(v.Owner.Name, v.Title, "audio", audio.MimeType))
			err = downloadFile(client, audioPath, append([]string{audio.BaseUrl}, audio.BackupUrl...))
			if err != nil {
				return err
			}

			mergeList = append(mergeList, VideoAudioPair{
				VideoPath:  videoPath,
				AudioPath:  audioPath,
				OutputPath: filepath.Join(outputPath, newFileName(v.Owner.Name, v.Title, "", "mp4")),
			})
		}

		ffmpeg := FFmpeg{Path: ffmpegPath}
		for _, m := range mergeList {
			zap.L().Info("Merging", zap.String("output", filepath.Base(m.OutputPath)))
			err = ffmpeg.MergeVideoAudio(m.VideoPath, m.AudioPath, m.OutputPath)
			if err != nil {
				return err
			}
			_ = os.Remove(m.VideoPath)
			_ = os.Remove(m.AudioPath)
		}

		return nil
	},
}

var RootCmd = &cli.Command{
	Name:    "bilibili",
	Aliases: []string{"bili", "b"},
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

	zap.L().Info("Downloading", zap.String("name", fileName))
	contentLength := getContentLength(rsp.Header())
	bar := newProgressBar(contentLength, "")
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

type FFmpeg struct {
	Path string
}

func (f *FFmpeg) MergeVideoAudio(videoPath, audioPath, outputPath string) error {
	cmd := exec.Command(f.Path, "-i", videoPath, "-i", audioPath, "-c:v", "copy", "-c:a", "copy", outputPath)
	buf, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(buf))
	}
	return nil
}
