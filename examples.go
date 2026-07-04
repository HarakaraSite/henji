package main

import "regexp"

// example is one entry shown in the --help footer.
type example struct {
	title, command string
}

// helpExamples are shown in --help, in this order, every time (not a random
// pick): one familiar pipe-and-summarize example for orientation, then two
// that cover the AI/scripted-consumption surface (--output json for a safe
// text envelope, --json-schema for a strictly-typed answer) since those are
// the features an agent invoking henji as a tool is least likely to discover
// from flag descriptions alone, then one showing henji composed with an
// existing CLI tool rather than performing file/tool access itself.
var helpExamples = []example{
	{
		title:   "Editorialize your video files",
		command: `ls ~/vids | henji -f "summarize each of these titles, group them by decade"`,
	},
	{
		title:   "Draft your commit message for you",
		command: `git diff --staged | henji --output json "suggest 3 commit messages for this diff" | jq -r '.content[0].text'`,
	},
	{
		title:   "Get a strictly-typed answer you can trust without parsing",
		command: `git diff main | henji --json-schema review-schema.json "review this diff for security issues" | jq '.findings[]'`,
	},
	{
		title:   "Investigate your project's disk usage",
		command: `du -sh * | sort -rh | henji "explain what's taking up the most space and why"`,
	},
}

func cheapHighlighting(s styles, code string) string {
	code = regexp.
		MustCompile(`"([^"\\]|\\.)*"`).
		ReplaceAllStringFunc(code, func(x string) string {
			return s.Quote.Render(x)
		})
	code = regexp.
		MustCompile(`\|`).
		ReplaceAllStringFunc(code, func(x string) string {
			return s.Pipe.Render(x)
		})
	return code
}
