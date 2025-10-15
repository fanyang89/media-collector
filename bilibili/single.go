package bilibili

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
)

var downloadSingleCmd = &cli.Command{
	Name:  "single",
	Usage: "Download a single video by BVID/AID",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "bvid"}, &cli.IntFlag{Name: "aid"},
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
		bvid := command.String("bvid")
		aid := command.Int("aid")
		if bvid == "" && aid == 0 {
			return errors.New("bvid/aid is required")
		}
		if aid != 0 {
			bvid = convertAidToBvid(aid)
		}

		d, err := downloaderFromCliCommand(command)
		if err != nil {
			return err
		}

		videoInfo, err := d.GetVideoInfo(bvid)
		if err != nil {
			return err
		}

		return d.Download(DownloadOption{
			Bvid:      videoInfo.Bvid,
			Cid:       videoInfo.Cid,
			OwnerName: videoInfo.Owner.Name,
			Title:     videoInfo.Title,
		}, false, true)
	},
}
