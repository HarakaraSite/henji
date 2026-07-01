# Mods Examples

### Improve Your Code

Piping source code to Mods and giving it an instruction on what to do with it
gives you a lot of options for refactoring, enhancing or debugging code.

`henji -f "what are your thoughts on improving this code?" < main.go | glow`

### Come Up With Product Features

Mods can also come up with entirely new features based on source code (or a
README file).

`henji -f "come up with 10 new features for this tool." < main.go | glow`

### Help Write Docs

Mods can quickly give you a first draft for new documentation.

`henji "write a new section to this readme for a feature that sends you a free rabbit if you hit r" < README.md | glow`

### Organize Your Videos

The file system can be an amazing source of input for Mods. If you have music
or video files, Mods can parse the output of `ls` and offer really good
editorialization of your content.

`ls ~/vids | henji -f "organize these by decade and summarize each" | glow`

### Make Recommendations

Mods is really good at generating recommendations based on what you have as
well, both for similar content but also content in an entirely different media
(like getting music recommendations based on movies you have).

`ls ~/vids | henji -f "recommend me 10 shows based on these, make them obscure" | glow`

`ls ~/vids | henji -f "recommend me 10 albums based on these shows, do not include any soundtrack music or music from the show" | glow`

### Read Your Fortune

It's easy to let your downloads folder grow into a chaotic never-ending pit of
files, but with Mods you can use that to your advantage!

`ls ~/Downloads | henji -f "tell my fortune based on these files" | glow`

### Understand APIs

Mods can parse and understand the output of an API call with `curl` and convert
it to something human readable.

`curl "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true&hourly=temperature_2m,relativehumidity_2m,windspeed_10m" 2>/dev/null | henji -f "summarize this weather data for a human." | glow`

### Read The Comments (so you don't have to)

Just like with APIs, Mods can read through raw HTML and summarize the contents.

`curl "https://news.ycombinator.com/item?id=30048332" 2>/dev/null | henji -f "what are the authors of these comments saying?" | glow`
