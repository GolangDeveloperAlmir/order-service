package log

import "go.uber.org/zap"

type Logger = zap.Logger

var (
	Any = zap.Any
	Err = zap.Error
	Str = zap.String
	Int = zap.Int
)

func New(env string) *Logger {
	if env == "prod" {
		l, err := zap.NewProduction()
		if err != nil {
			l.Error("failed to create logger", zap.Error(err))
		}

		return l
	}
	l, err := zap.NewDevelopment()
	if err != nil {
		l.Error("failed to create logger", zap.Error(err))
	}

	return l
}
