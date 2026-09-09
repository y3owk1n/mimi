package config

import (
	"strconv"
	"strings"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Color is a color as the config spells it, each channel from 0 to 1.
type Color struct {
	Red, Green, Blue, Alpha float64
}

// channelMax is the largest value a two-digit hex channel holds.
const channelMax = 255

// ParseColor reads a color written as #rrggbb or #rrggbbaa, the leading #
// optional. The alpha is 1 when left out.
func ParseColor(text string) (Color, error) {
	hex := strings.TrimPrefix(strings.TrimSpace(text), "#")
	if len(hex) != 6 && len(hex) != 8 {
		return Color{}, derrors.Newf(
			derrors.CodeInvalidConfig,
			"color %q must be #rrggbb or #rrggbbaa",
			text,
		)
	}

	var channels [4]float64

	channels[3] = 1

	for index := range len(hex) / 2 {
		value, err := strconv.ParseUint(hex[index*2:index*2+2], 16, 8)
		if err != nil {
			return Color{}, derrors.Newf(
				derrors.CodeInvalidConfig,
				"color %q must be #rrggbb or #rrggbbaa",
				text,
			)
		}

		channels[index] = float64(value) / channelMax
	}

	return Color{Red: channels[0], Green: channels[1], Blue: channels[2], Alpha: channels[3]}, nil
}
