package bilibili

import (
	"context"
	"strings"

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
	},
	Action: func(ctx context.Context, command *cli.Command) error {
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

			for _, result := range rsp.Result {
				if result.ResultType != "video" {
					continue
				}
				zap.L().Info("Search", zap.Int("page", page), zap.Int("count", len(result.Data)))
				for _, m := range result.Data {
					results = append(results, NewVideoSearchResult(m))
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
				return err
			}
		}

		return nil
	},
}

type VideoSearchResult struct {
	Bvid   string   `json:"bvid"`
	Author string   `json:"author"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags"`
}

func NewVideoSearchResult(m map[string]any) *VideoSearchResult {
	return &VideoSearchResult{
		Bvid:   m["bvid"].(string),
		Author: m["author"].(string),
		Title:  getInnerText(m["title"].(string)),
		Tags:   strings.Split(m["tag"].(string), ","),
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
