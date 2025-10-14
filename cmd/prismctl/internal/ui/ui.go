// Package ui provides visual feedback components for prismctl
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for consistent UI
var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	subtleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
)

// UI provides console output helpers
type UI struct {
	out io.Writer
	err io.Writer
}

// NewUI creates a new UI instance
func NewUI() *UI {
	return &UI{
		out: os.Stdout,
		err: os.Stderr,
	}
}

// Success prints a success message
func (ui *UI) Success(msg string) {
	fmt.Fprintln(ui.out, successStyle.Render("✓ "+msg))
}

// Error prints an error message
func (ui *UI) Error(msg string) {
	fmt.Fprintln(ui.err, errorStyle.Render("✗ "+msg))
}

// Warning prints a warning message
func (ui *UI) Warning(msg string) {
	fmt.Fprintln(ui.out, warningStyle.Render("⚠ "+msg))
}

// Info prints an info message
func (ui *UI) Info(msg string) {
	fmt.Fprintln(ui.out, infoStyle.Render("ℹ "+msg))
}

// Subtle prints a subtle/muted message
func (ui *UI) Subtle(msg string) {
	fmt.Fprintln(ui.out, subtleStyle.Render(msg))
}

// Println prints a regular message
func (ui *UI) Println(msg string) {
	fmt.Fprintln(ui.out, msg)
}

// Printf prints a formatted message
func (ui *UI) Printf(format string, args ...interface{}) {
	fmt.Fprintf(ui.out, format, args...)
}

// Header prints a section header
func (ui *UI) Header(title string) {
	fmt.Fprintln(ui.out, headerStyle.Render(title))
}

// Separator prints a visual separator
func (ui *UI) Separator() {
	fmt.Fprintln(ui.out, subtleStyle.Render(strings.Repeat("─", 60)))
}

// KeyValue prints a key-value pair
func (ui *UI) KeyValue(key, value string) {
	fmt.Fprintf(ui.out, "  %s: %s\n", subtleStyle.Render(key), value)
}

// ListItem prints a list item
func (ui *UI) ListItem(item string) {
	fmt.Fprintln(ui.out, "  • "+item)
}

// Table prints a simple table
type Table struct {
	ui      *UI
	headers []string
	rows    [][]string
}

// NewTable creates a new table
func (ui *UI) NewTable(headers ...string) *Table {
	return &Table{
		ui:      ui,
		headers: headers,
		rows:    make([][]string, 0),
	}
}

// AddRow adds a row to the table
func (t *Table) AddRow(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Render renders the table
func (t *Table) Render() {
	if len(t.headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(t.headers))
	for i, header := range t.headers {
		widths[i] = len(header)
	}

	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	headerParts := make([]string, len(t.headers))
	for i, header := range t.headers {
		headerParts[i] = padRight(header, widths[i])
	}
	t.ui.Println(headerStyle.Render(strings.Join(headerParts, " | ")))

	// Print separator
	separatorParts := make([]string, len(widths))
	for i, width := range widths {
		separatorParts[i] = strings.Repeat("─", width)
	}
	t.ui.Println(subtleStyle.Render(strings.Join(separatorParts, "─┼─")))

	// Print rows
	for _, row := range t.rows {
		rowParts := make([]string, len(t.headers))
		for i := 0; i < len(t.headers); i++ {
			if i < len(row) {
				rowParts[i] = padRight(row[i], widths[i])
			} else {
				rowParts[i] = padRight("", widths[i])
			}
		}
		t.ui.Println(strings.Join(rowParts, " │ "))
	}
}

// padRight pads a string to the right with spaces
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// DeviceCodePrompt displays device code authentication prompt
func (ui *UI) DeviceCodePrompt(verificationURI, userCode string, expiresIn int) {
	ui.Println("")
	ui.Header("🔐 Prism Authentication")
	ui.Println("")
	ui.Info("1. Open this URL in your browser:")
	ui.Println("   " + verificationURI)
	ui.Println("")
	ui.Info("2. Enter this code:")
	ui.Println("   " + successStyle.Render(userCode))
	ui.Println("")
	ui.Subtle(fmt.Sprintf("Waiting for authentication (code expires in %ds)...", expiresIn))
	ui.Println("")
}
