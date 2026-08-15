//nolint:testpackage
package cmd

import (
	"testing"

	"github.com/y3owk1n/mimi/internal/service"
)

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name   string
		status service.Status
		want   string
	}{
		{name: "loaded", status: service.Status{Loaded: true}, want: "Service loaded"},
		{name: "not loaded", status: service.Status{Loaded: false}, want: "Service not loaded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStatus(tt.status)
			if got != tt.want {
				t.Errorf("formatStatus(%+v) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
