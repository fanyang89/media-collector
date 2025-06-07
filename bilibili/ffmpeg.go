package bilibili

import (
	"os/exec"

	"github.com/cockroachdb/errors"
)

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
