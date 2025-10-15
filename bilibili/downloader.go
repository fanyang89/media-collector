package bilibili

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/CuteReimu/bilibili/v2"
)

type Downloader struct {
	ffmpeg      FFmpeg
	outputPath  string
	client      *bilibili.Client
	configPath  string
	config      *Config
	history     *History
	rateLimiter *rate.Limiter
	maxFileSize int64
}

func downloaderFromCliCommand(command *cli.Command) (*Downloader, error) {
	return newDownloader(command.String("config"))
}

func NewDownloaderFromConfig(config *Config) *Downloader {
	b := bilibili.New()
	b.SetCookiesString(config.Cookies)
	return &Downloader{
		config:      config,
		ffmpeg:      FFmpeg{Path: config.FFmpeg},
		outputPath:  config.Output,
		rateLimiter: rate.NewLimiter(rate.Every(time.Second), 1),
		client:      b,
	}
}

func newDownloader(configPath string) (*Downloader, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	if config.Cookies == "" {
		return nil, errors.New("please login first")
	}
	d := &Downloader{
		configPath: configPath,
		config:     config,
	}

	history, err := NewHistory(config.HistoryDB)
	if err != nil {
		return nil, err
	}
	d.history = history

	ffmpegPath := config.FFmpeg
	_, err = os.Stat(ffmpegPath)
	if err != nil {
		return nil, errors.Wrap(err, "ffmpeg not exist, please install ffmpeg first")
	}
	d.ffmpeg = FFmpeg{Path: ffmpegPath}

	outputPath := config.Output
	_, err = os.Stat(outputPath)
	if err != nil && os.IsNotExist(err) {
		err = os.Mkdir(outputPath, 0755)
		if err != nil {
			return nil, err
		}
	}
	d.outputPath = outputPath

	d.client = bilibili.New()
	d.client.SetCookiesString(config.Cookies)

	d.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)
	return d, nil
}

func (d *Downloader) GetVideoInfo(bvid string) (*bilibili.VideoInfo, error) {
	return d.GetClient().GetVideoInfo(bilibili.VideoParam{Bvid: bvid})
}

func (d *Downloader) GetClient() *bilibili.Client {
	_ = d.rateLimiter.Wait(context.Background())
	time.Sleep(time.Duration(rand.IntN(3)+1) * time.Second)
	return d.client
}

type StreamType string

const (
	Video StreamType = "video"
	Audio            = "audio"
)

func getFileName(option DownloadOption, videoOrAudio *bilibili.AudioOrVideo, streamType StreamType) string {
	if videoOrAudio == nil {
		return newFileName(option.OwnerName, option.Title, "", "mp4")
	}
	switch streamType {
	case Audio:
		return newFileName(option.OwnerName, option.Title, "audio", videoOrAudio.MimeType)
	case Video:
		return newFileName(option.OwnerName, option.Title, "video", videoOrAudio.MimeType)
	}
	panic("invalid arguments")
}

type DownloadOption struct {
	Bvid             string
	Cid              int
	OwnerName        string
	Title            string
	SearchKeyword    string
	Tags             []string
	DownloadProgress string
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	slog.Error("failed to check if file exists", zap.String("filePath", filePath))
	return false
}

func (d *Downloader) Download(option DownloadOption, force bool, saveHistory bool) error {
	if !force {
		ok, err := d.history.IsDownloaded(option.Bvid)
		if err != nil {
			return err
		}
		if ok {
			zap.L().Info("Already downloaded", zap.String("bvid", option.Bvid),
				zap.String("owner", option.OwnerName), zap.String("title", option.Title))
			return nil
		}
	}

	var err error

	if option.Cid == 0 {
		var videoInfo *bilibili.VideoInfo
		videoInfo, err = d.GetClient().GetVideoInfo(bilibili.VideoParam{Bvid: option.Bvid})
		if err != nil {
			return err
		}
		option.Cid = videoInfo.Cid
	}

	result, err := d.GetClient().GetVideoStream(NewGetVideoStreamParam(option.Bvid, option.Cid))
	if err != nil {
		return errors.Wrapf(err, "get video stream, bvid: %s, cid: %d", option.Bvid, option.Cid)
	}
	if len(result.Dash.Video) == 0 || len(result.Dash.Audio) == 0 {
		if result.Result == "suee" {
			zap.L().Info("Not available streams", zap.String("bvid", option.Bvid))
			return nil
		}
		return errors.Newf("can't get video stream, bvid: %s", option.Bvid)
	}

	slices.SortFunc(result.Dash.Video, func(a, b bilibili.AudioOrVideo) int { return b.Bandwidth - a.Bandwidth })
	slices.SortFunc(result.Dash.Audio, func(a, b bilibili.AudioOrVideo) int { return b.Bandwidth - a.Bandwidth })

	outputFile := getFileName(option, nil, Video)
	dstFilePath := filepath.Join(d.outputPath, outputFile)
	if fileExists(dstFilePath) {
		slog.Info("Skip download", "fileName", outputFile)
		return nil
	}

	video := result.Dash.Video[0]
	videoPath := filepath.Join(d.outputPath, newFileName(option.OwnerName, option.Title, "video", video.MimeType))

	err = d.DownloadFile(videoPath, append([]string{video.BaseUrl}, video.BackupUrl...))
	if err != nil {
		return err
	}

	audio := result.Dash.Audio[0]
	audioPath := filepath.Join(d.outputPath, newFileName(option.OwnerName, option.Title, "audio", audio.MimeType))

	err = d.DownloadFile(audioPath, append([]string{audio.BaseUrl}, audio.BackupUrl...))
	if err != nil {
		return err
	}

	if option.DownloadProgress != "" {
		fmt.Printf("%s Merging %s\n", option.DownloadProgress, outputFile)
	} else {
		fmt.Printf("Merging %s\n", outputFile)
	}

	ffmpeg := d.ffmpeg
	err = ffmpeg.MergeVideoAudio(videoPath, audioPath, filepath.Join(d.outputPath, outputFile))
	if err != nil {
		zap.L().Error("Merge failed", zap.Error(err), zap.String("file", outputFile))
		return nil
	}

	_ = os.Remove(videoPath)
	_ = os.Remove(audioPath)

	if saveHistory {
		return d.history.Save(&HistoryEntry{
			Bvid:     option.Bvid,
			Author:   option.OwnerName,
			Title:    option.Title,
			Keyword:  option.SearchKeyword,
			Tags:     strings.Join(option.Tags, ";"),
			FileName: outputFile,
		})
	}

	return nil
}

func (d *Downloader) SaveConfig() error {
	cookies := d.client.GetCookiesString()
	d.config.Cookies = cookies
	return SaveConfig(d.configPath, d.config)
}
