package filesystem

import (
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
)

var defaultFormatter = &logrus.TextFormatter{DisableColors: true}

type FilesystemHook struct {
	path string
	hook *lfshook.LfsHook
}

func New(path string) (*FilesystemHook, error) {
	writer, err := rotatelogs.New(
		path+".%Y%m%d%H%M",
		rotatelogs.WithLinkName(path),
		rotatelogs.WithMaxAge(time.Duration(86400)*time.Second),
		rotatelogs.WithRotationTime(time.Duration(604800)*time.Second),
	)

	if err != nil {
		return nil, err
	}

	return &FilesystemHook{
		path: path,
		hook: lfshook.NewHook(
			lfshook.WriterMap{
				logrus.PanicLevel: writer,
				logrus.FatalLevel: writer,
				logrus.ErrorLevel: writer,
				logrus.WarnLevel:  writer,
				logrus.InfoLevel:  writer,
				logrus.DebugLevel: writer,
			},
			defaultFormatter,
		),
	}, nil
}

func (h *FilesystemHook) Fire(entry *logrus.Entry) error {
	return h.hook.Fire(entry)
}

// Levels returns the available logging levels.
func (h *FilesystemHook) Levels() []logrus.Level {
	return h.hook.Levels()
}
