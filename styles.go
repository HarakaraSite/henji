package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/colorprofile"
)

type styles struct {
	AppName,
	CliArgs,
	Comment,
	CyclingChars,
	ErrorHeader,
	ErrorDetails,
	ErrPadding,
	Flag,
	FlagComma,
	FlagDesc,
	InlineCode,
	Link,
	Pipe,
	Quote,
	ConversationList,
	SHA1,
	Timeago lipgloss.Style
	profile colorprofile.Profile
}

func makeStyles(profile colorprofile.Profile) (s styles) {
	const horizontalEdgePadding = 2
	colorForProfile := func(color string) color.Color {
		return profile.Convert(lipgloss.Color(color))
	}
	adaptiveColor := func(light, dark string) color.Color {
		return profile.Convert(compat.AdaptiveColor{
			Light: lipgloss.Color(light),
			Dark:  lipgloss.Color(dark),
		})
	}
	bold := func(style lipgloss.Style) lipgloss.Style {
		return style.Bold(profile > colorprofile.ASCII)
	}
	s.profile = profile
	s.AppName = bold(lipgloss.NewStyle())
	s.CliArgs = lipgloss.NewStyle().Foreground(colorForProfile("#585858"))
	s.Comment = lipgloss.NewStyle().Foreground(colorForProfile("#757575"))
	s.CyclingChars = lipgloss.NewStyle().Foreground(colorForProfile("#FF87D7"))
	s.ErrorHeader = bold(lipgloss.NewStyle().Foreground(colorForProfile("#F1F1F1")).Background(colorForProfile("#FF5F87"))).Padding(0, 1).SetString("ERROR")
	s.ErrorDetails = s.Comment
	s.ErrPadding = lipgloss.NewStyle().Padding(0, horizontalEdgePadding)
	s.Flag = bold(lipgloss.NewStyle().Foreground(adaptiveColor("#00B594", "#3EEFCF")))
	s.FlagComma = lipgloss.NewStyle().Foreground(adaptiveColor("#5DD6C0", "#427C72")).SetString(",")
	s.FlagDesc = s.Comment
	s.InlineCode = lipgloss.NewStyle().Foreground(colorForProfile("#FF5F87")).Background(colorForProfile("#3A3A3A")).Padding(0, 1)
	s.Link = lipgloss.NewStyle().Foreground(colorForProfile("#00AF87")).Underline(profile > colorprofile.ASCII)
	s.Quote = lipgloss.NewStyle().Foreground(adaptiveColor("#FF71D0", "#FF78D2"))
	s.Pipe = lipgloss.NewStyle().Foreground(adaptiveColor("#8470FF", "#745CFF"))
	s.ConversationList = lipgloss.NewStyle().Padding(0, 1)
	s.SHA1 = s.Flag
	s.Timeago = lipgloss.NewStyle().Foreground(adaptiveColor("#999", "#555"))
	return s
}

// action messages

const defaultAction = "WROTE"

func printConfirmation(action, content string) {
	if action == "" {
		action = defaultAction
	}
	profile := stdoutStyles().profile
	outputHeader := lipgloss.NewStyle().
		Foreground(profile.Convert(lipgloss.Color("#F1F1F1"))).
		Background(profile.Convert(lipgloss.Color("#6C50FF"))).
		Bold(profile > colorprofile.ASCII).
		Padding(0, 1).
		MarginRight(1).
		SetString(strings.ToUpper(action))
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Center, outputHeader.String(), content))
}
