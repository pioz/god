package runner

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Types of messages. Types are normal, success and error.
type MessageStatus uint8

const (
	// Normal message (uncolored)
	MessageNormal MessageStatus = 1 << iota // 1 << 0 which is 00000001
	// Success message (green)
	MessageSuccess // 1 << 1 which is 00000010
	// Error message (red)
	MessageError // 1 << 2 which is 00000100
	// Warning message (yellow)
	MessageWarning // 1 << 3 which is 00001000
)

type message struct {
	serviceName string
	text        string
	status      MessageStatus
}

const (
	green1  = "#22c55e"
	green2  = "#059669"
	red1    = "#ef4444"
	red2    = "#dc2626"
	yellow1 = "#f9c10b"
	yellow2 = "#fcc203"
)

var styles = map[MessageStatus]map[string]lipgloss.Style{
	MessageNormal: {
		"normal": lipgloss.NewStyle(),
		"bold":   lipgloss.NewStyle().Bold(true),
		"symbol": lipgloss.NewStyle().SetString("→"),
	},
	MessageSuccess: {
		"normal": lipgloss.NewStyle().Foreground(lipgloss.Color(green2)),
		"bold":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(green1)),
		"symbol": lipgloss.NewStyle().SetString("✓").Foreground(lipgloss.Color(green1)),
	},
	MessageError: {
		"normal": lipgloss.NewStyle().Foreground(lipgloss.Color(red2)),
		"bold":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(red1)),
		"symbol": lipgloss.NewStyle().SetString("×").Foreground(lipgloss.Color(red1)),
	},
	MessageWarning: {
		"normal": lipgloss.NewStyle().Foreground(lipgloss.Color(yellow2)),
		"bold":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(yellow1)),
		"symbol": lipgloss.NewStyle().SetString("⚠").Foreground(lipgloss.Color(yellow1)),
	},
}

func (m *message) print(width int) {

	if styles[m.status] == nil {
		return
	}
	if len(m.serviceName) > width {
		width = len(m.serviceName)
	}
	if width != 0 {
		width += 3 // add padding
	}
	if m.text == "" && m.status == MessageSuccess {
		m.text = "ok"
	}
	fmt.Println(lipgloss.JoinHorizontal(
		lipgloss.Top,
		styles[m.status]["symbol"].String(),
		styles[m.status]["bold"].PaddingLeft(1).Width(width).Render("["+m.serviceName+"]"),
		styles[m.status]["normal"].PaddingLeft(1).Width(120).Render(m.text),
	))
}
