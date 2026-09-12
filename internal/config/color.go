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
const (
	channelMax = 255
	// rgbChannels and argbChannels are how many channels a color is
	// written with, without and with its alpha.
	rgbChannels  = 3
	argbChannels = 4
)

// ParseColor reads a color written as #rrggbb or #aarrggbb, the leading #
// optional. The alpha comes first when given, as the Android and Compose
// conventions write it, and is 1 when left out.
func ParseColor(text string) (Color, error) {
	hex := strings.TrimPrefix(strings.TrimSpace(text), "#")
	if len(hex) != 6 && len(hex) != 8 {
		return Color{}, derrors.Newf(
			derrors.CodeInvalidConfig,
			"color %q must be #rrggbb or #aarrggbb",
			text,
		)
	}

	// Channels in the order written, alpha first when there are four.
	values := make([]float64, 0, argbChannels)

	for index := range len(hex) / 2 {
		value, err := strconv.ParseUint(hex[index*2:index*2+2], 16, 8)
		if err != nil {
			return Color{}, derrors.Newf(
				derrors.CodeInvalidConfig,
				"color %q must be #rrggbb or #aarrggbb",
				text,
			)
		}

		values = append(values, float64(value)/channelMax)
	}

	if len(values) == rgbChannels {
		values = append([]float64{1}, values...)
	}

	return Color{Alpha: values[0], Red: values[1], Green: values[2], Blue: values[3]}, nil
}
