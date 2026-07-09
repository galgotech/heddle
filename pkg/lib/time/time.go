package time

import (
	"time"

	"heddle/pkg/runtime/interaction"
)

func Now() string {
	return time.Now().Format(time.RFC3339)
}

type TimeTag string

const (
	TagTick TimeTag = "tick"
)

func Tick(durStr string) (interaction.Seq[TimeTag, string], error) {
	d, err := time.ParseDuration(durStr)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(d)
	return func(yield func(interaction.Match[TimeTag, string]) bool) {
		for t := range ticker.C {
			matchVal := interaction.Match[TimeTag, string]{
				Tag: TagTick,
				Val: t.Format(time.RFC3339),
			}
			if !yield(matchVal) {
				ticker.Stop()
				break
			}
		}
	}, nil
}
