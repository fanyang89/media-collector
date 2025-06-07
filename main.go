package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/fanyang89/media-collector/bilibili"
)

var cmd = &cli.Command{
	Name:  "media-collector",
	Usage: "Media collector",
	Commands: []*cli.Command{
		bilibili.RootCmd,
	},
}

func main() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger, err := config.Build(zap.AddCaller(), zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)
	defer func() { _ = logger.Sync() }()

	err = cmd.Run(context.Background(), os.Args)
	if err != nil {
		zap.L().Error("Unexpected error", zap.Error(err))
	}
}
