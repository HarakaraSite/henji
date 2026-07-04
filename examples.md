# henji Examples

### Improve Your Code

Piping source code to henji and giving it an instruction on what to do with it
gives you a lot of options for refactoring, enhancing or debugging code.

`henji -f "what are your thoughts on improving this code?" < main.go`

### Come Up With Product Features

henji can also come up with entirely new features based on source code (or a
README file).

`henji -f "come up with 10 new features for this tool." < main.go`

### Help Write Docs

henji can quickly give you a first draft for new documentation.

`henji "write a new section to this readme for a feature that sends you a free rabbit if you hit r" < README.md`

### Organize Your Videos

The file system can be an amazing source of input for henji. If you have music
or video files, henji can parse the output of `ls` and offer really good
editorialization of your content.

`ls ~/vids | henji -f "organize these by decade and summarize each"`

### Make Recommendations

henji is really good at generating recommendations based on what you have as
well, both for similar content but also content in an entirely different media
(like getting music recommendations based on movies you have).

`ls ~/vids | henji -f "recommend me 10 shows based on these, make them obscure"`

`ls ~/vids | henji -f "recommend me 10 albums based on these shows, do not include any soundtrack music or music from the show"`

### Read Your Fortune

It's easy to let your downloads folder grow into a chaotic never-ending pit of
files, but with henji you can use that to your advantage!

`ls ~/Downloads | henji -f "tell my fortune based on these files"`

### Understand APIs

henji can parse and understand the output of an API call with `curl` and convert
it to something human readable.

`curl "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true&hourly=temperature_2m,relativehumidity_2m,windspeed_10m" 2>/dev/null | henji -f "summarize this weather data for a human."`

### Read The Comments (so you don't have to)

Just like with APIs, henji can read through raw HTML and summarize the contents.

`curl "https://news.ycombinator.com/item?id=30048332" 2>/dev/null | henji -f "what are the authors of these comments saying?"`

### Draft Your Commit Message For You

Pipe a staged diff to henji and get commit message candidates back. Use
`--output json` so a git hook or script can embed the suggestion without
having to parse plain text.

`git diff --staged | henji --output json "suggest 3 commit messages for this diff" | jq -r '.content[0].text'`

### Make Sense of a Giant Log File Without Opening It

Server logs balloon fast. Hand the whole thing to henji and get back just the
distilled pattern summary, without loading it into your editor (or an AI
agent's own context) first.

`cat huge_server.log | henji --output json "find the top 5 error patterns and count each" | jq -r '.content[0].text'`

### Try a Free Local Model Before Paying for a Cloud One

Ask a local gateway first, and only escalate to a paid cloud model if you're
not satisfied with the answer.

`ollama` here is just the name given to the API in `henji.yml`; henji talks
to it over Ollama's OpenAI-compatible `/v1` endpoint, not a dedicated client.

`echo "explain this regex: ^(?:[a-z0-9]+_)+[a-z0-9]+$" | henji --api ollama --model llama3.2 -f`

### Have henji Investigate Your Project's Bloat

With an MCP filesystem server configured, henji can actually inspect your
disk instead of guessing from memory, and explain what it finds.

`henji --max-tool-calls 5 "list the largest files in my current project and explain what each is for"`

### Get a Rigorously Reasoned Answer to a Hard Problem

Reasoning models like DeepSeek R1 think before they answer, which shows in
noticeably more careful math and logic responses.

`henji --api deepseek --model r1 --max-tokens 4096 "prove there are infinitely many primes, then poke holes in your own proof"`

### Try Multiple Solutions From the Same Starting Point

Branch a conversation to get several distinct takes on the same problem
without re-explaining it each time.

```
henji --title "api-design" "sketch a REST API for a todo app"
henji --continue "api-design" --title "graphql-take" "now redo it as GraphQL"
```
