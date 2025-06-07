package bilibili

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"golang.org/x/net/html"

	"github.com/CuteReimu/bilibili/v2"
)

var downloadSearchCmd = &cli.Command{
	Name:  "search",
	Usage: "Search and download videos",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "keyword", Config: cli.StringConfig{TrimSpace: true}},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yml",
		},
		&cli.IntFlag{
			Name:    "max-items",
			Aliases: []string{"max", "m"},
			Value:   200,
		},
		&cli.DurationFlag{
			Name:  "max-duration",
			Value: time.Hour,
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		maxDuration := command.Duration("max-duration")
		keyword := command.StringArg("keyword")
		if keyword == "" {
			return errors.New("keyword is required")
		}

		d, err := downloaderFromCliCommand(command)
		if err != nil {
			return err
		}

		maxItems := command.Int("max-items")
		results := make([]*VideoSearchResult, 0)
		page := 1

		for len(results) < maxItems {
			rsp, err := d.GetClient().IntergratedSearch(bilibili.SearchParam{
				Keyword: keyword,
				Page:    page,
			})
			if err != nil {
				return err
			}
			if rsp == nil {
				zap.L().Info("Search response is nil")
				break
			}

			for _, result := range rsp.Result {
				if result.ResultType != "video" {
					continue
				}
				zap.L().Info("Search", zap.Int("page", page), zap.Int("count", len(result.Data)))
				for _, m := range result.Data {
					r := NewVideoSearchResult(m)
					if r.IsPay {
						zap.L().Info("Skip paid video", zap.String("bvid", r.Bvid),
							zap.String("title", r.Title))
						continue
					}

					ok, err := d.history.IsDownloaded(r.Bvid)
					if err != nil {
						return err
					}
					if ok {
						continue
					}

					if maxDuration <= time.Duration(0) {
						results = append(results, r)
					} else if r.Duration <= maxDuration {
						results = append(results, r)
					} else {
						zap.L().Info("Skip long video", zap.String("bvid", r.Bvid),
							zap.String("title", r.Title), zap.Duration("duration", r.Duration))
					}
				}
			}

			page++
		}

		zap.L().Info("Search completed", zap.Int("results", len(results)))

		for _, r := range results {
			err = d.Download(DownloadOption{
				Bvid:          r.Bvid,
				OwnerName:     r.Author,
				Title:         r.Title,
				SearchKeyword: keyword,
				Tags:          r.Tags,
			}, false)
			if err != nil {
				zap.L().Error("Download failed", zap.String("bvid", r.Bvid), zap.Error(err))
				continue
			}
		}

		return nil
	},
}

type VideoSearchResult struct {
	Bvid     string        `json:"bvid"`
	Author   string        `json:"author"`
	Title    string        `json:"title"`
	Tags     []string      `json:"tags"`
	Duration time.Duration `json:"duration"`
	IsPay    bool          `json:"is_pay"`
}

func parseDuration(s string) time.Duration {
	var err error
	d := time.Duration(0)
	parts := strings.Split(s, ":")
	var h, m, sec int64
	if len(parts) == 2 {
		m, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			panic(err)
		}
		sec, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			panic(err)
		}
	} else if len(parts) == 3 {
		h, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			panic(err)
		}
		m, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			panic(err)
		}
		sec, err = strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			panic(err)
		}
	} else {
		panic(errors.Newf("invalid duration: %s", s))
	}

	d += time.Duration(h) * time.Hour
	d += time.Duration(m) * time.Minute
	d += time.Duration(sec) * time.Second
	return d
}

func NewVideoSearchResult(m map[string]any) *VideoSearchResult {
	durationStr := m["duration"].(string)
	return &VideoSearchResult{
		Bvid:     m["bvid"].(string),
		Author:   m["author"].(string),
		Title:    getInnerText(m["title"].(string)),
		Tags:     strings.Split(m["tag"].(string), ","),
		Duration: parseDuration(durationStr),
		IsPay:    m["is_pay"].(float64) != 0,
	}
}

func getInnerText(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	return extractText(doc)
}

func extractText(n *html.Node) string {
	var b strings.Builder
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(extractText(c))
	}
	return b.String()
}
