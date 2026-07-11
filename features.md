# henji Features

## Regular usage

By default:

- model responses go to `STDOUT`; progress and status messages go to `STDERR`
- successful model conversations are saved with the first prompt line as the
  title, unless `--no-cache` is set
- glamour is used by default if `STDOUT` is a TTY
- a small `Generating` spinner is shown on `STDERR` while a TTY request waits

### Basic

The most basic usage is:

```bash
henji 'first 2 primes'
```

### Pipe from

You can also pipe to it, in which case `STDIN` will not be a TTY:

```bash
echo 'as json' | henji 'first 2 primes'
```

In this case, `henji` should read `STDIN` and append it to the prompt.

### Pipe to

You may also pipe the output to another program, in which case `STDOUT` will not
be a TTY:

```bash
echo 'as json' | henji 'first 2 primes' | jq .
```

In this case the response is streamed to `STDOUT`; the spinner is suppressed
because stdout is not a TTY.

### Custom title

You can set a custom title:

```bash
henji --title='title' 'first 2 primes'
```

### Continue a specific conversation

You can continue a named conversation and save the new turn with a new title:

```bash
henji --title='primes' 'first 2 primes'
henji --continue='primes' --title='primes as json' 'format as json'
```

### Untitled continue latest

```bash
henji 'first 2 primes'
henji --continue-last 'format as json'
```

### Continue from specific conversation, save with a new title

```bash
henji --title='naturals' 'first 5 natural numbers'
henji --continue='naturals' --title='naturals.json' 'format as json'
```

### Conversation branching

You can use the `--continue` and `--title` to branch out conversations, for
instance:

```bash
henji --title='naturals' 'first 5 natural numbers'
henji --continue='naturals' --title='naturals.json' 'format as json'
henji --continue='naturals' --title='naturals.yaml' 'format as yaml'
```

With this you'll end up with 3 conversations: `naturals`, `naturals.json`, and
`naturals.yaml`.

## List conversations

You can list your previous conversations with:

```bash
henji --list
# or
henji -l
```

The command always prints a tab-separated list. Pick an ID or title from that
output and pass it to `--show`, `--continue`, or `--delete`; there is no
interactive selector.

## Show a previous conversation

You can also show a previous conversation by ID or title, e.g.:

```bash
henji --show='naturals'
henji -s='a2e2'
```

For titles, the match should be exact.
For IDs, only the first 4 chars are needed. If it matches multiple
conversations, you can add more chars until it matches a single one again.

## Delete a conversation

You can also delete conversations by title or ID, same as `--show`, different
flag:

```bash
henji --delete='naturals' --delete='a2e2'
```

Keep in mind that these operations are not reversible.
You can repeat the delete flag to delete multiple conversations at once.
