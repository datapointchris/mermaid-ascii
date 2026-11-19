package render

import (
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/mattn/go-runewidth"
)

// WidthStatus reports graph width fitting.
type WidthStatus struct {
	Requested bool
	Limit     int
	Width     int
	Compacted bool
	Met       bool
}

func RenderDiagram(input string, config *diagram.Config) (string, error) {
	output, _, err := RenderDiagramWithStatus(input, config)
	return output, err
}

// Report carries what a render found beside its output: how the graph fit
// its width, and the places its layout could be misread.
type Report struct {
	Width          WidthStatus
	LayoutWarnings []string
}

// RenderDiagramWithStatus reports graph width fitting alongside the output.
func RenderDiagramWithStatus(input string, config *diagram.Config) (string, WidthStatus, error) {
	output, report, err := RenderDiagramWithReport(input, config)
	return output, report.Width, err
}

// RenderDiagramWithReport renders a diagram and reports its width fitting
// and layout warnings.
func RenderDiagramWithReport(input string, config *diagram.Config) (string, Report, error) {
	if config == nil {
		config = diagram.DefaultConfig()
	}

	// YAML frontmatter carries a title and theme config; the config has no
	// ASCII meaning, but the title is printed above the diagram like mermaid
	// does. Stripped here once so type detection and parsing never see it.
	input, title := diagram.StripFrontmatter(input)

	diag, err := DiagramFactory(input)
	if err != nil {
		return "", Report{}, fmt.Errorf("failed to detect diagram type: %w", err)
	}

	if err := diag.Parse(input); err != nil {
		return "", Report{}, fmt.Errorf("failed to parse %s diagram: %w", diag.Type(), err)
	}

	output, err := diag.Render(config)
	if err != nil {
		return "", Report{}, fmt.Errorf("failed to render %s diagram: %w", diag.Type(), err)
	}

	if title != "" {
		output = title + "\n\n" + output
	}

	report := Report{}
	if provider, ok := diag.(interface{ WidthStatus() WidthStatus }); ok {
		report.Width = provider.WidthStatus()
		report.Width.Width = displayWidth(output)
		report.Width.Met = report.Width.Requested && report.Width.Width <= report.Width.Limit
	}
	if provider, ok := diag.(interface{ LayoutWarnings() []string }); ok {
		report.LayoutWarnings = provider.LayoutWarnings()
	}
	return output, report, nil
}

func displayWidth(output string) int {
	width := 0
	for _, line := range strings.Split(output, "\n") {
		if lineWidth := runewidth.StringWidth(line); lineWidth > width {
			width = lineWidth
		}
	}
	return width
}
