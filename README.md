# ylint

A small YAML style linter. It reports problems with a file and line
number, one finding per line, so it's easy to pipe into an editor or a
CI job.

## Why

YAML has no braces or semicolons to catch a misplaced character, so
the usual failure mode is a file that parses fine but means something
different from what you intended: a key silently overwritten further
down the same mapping, a line indented with a tab that some parsers
accept and others reject, a config file that grew a 400-character
line nobody will ever read in a diff. Full YAML linters exist, but
they're generally a Python install away and need to build the whole
document tree before they can tell you anything. This one is a single
static binary, has no dependencies, and streams its input a line at a
time, so it works the same whether you're checking a five-line config
or tailing a multi-gigabyte generated file.

## Checks in this version

- `no-tabs` -- tab characters used for indentation
- `trailing-whitespace` -- trailing spaces or tabs at end of line
- `line-length` -- line longer than the configured maximum (120 by default)
- `duplicate-key` -- the same key defined twice in one mapping, including
  inside a flow-style mapping like `{name: api, name: web}`

## Build

```
go build -o ylint .
```

## Usage

```
ylint config.yaml
ylint --max-line-length 80 config.yaml
cat config.yaml | ylint
```

Given a file like this (the fourth line starts with a tab, not spaces):

```yaml
service:
  name: api
  name: web
	description: this line is deliberately way too long just to show what the line-length check catches when someone pastes a huge value in
```

running `ylint config.yaml` prints:

```
config.yaml:3: duplicate-key: key "name" already defined on line 2
config.yaml:4: no-tabs: tab used in indentation
config.yaml:4: line-length: line is 136 characters, exceeds 120
```

Exit code is `1` if any findings were reported, `2` on a read error,
and `0` if the input is clean.

## Status

Early. It reads plain scalars and block mappings well enough to catch
the checks above, and now recognizes flow-style mappings and sequences
(`{a: 1}`, `[1, 2]`) so their contents don't get misread as block keys
and duplicate keys inside a flow mapping are caught too. A flow
collection that spans more than one line isn't understood yet, and
there's still no support for anchors or multi-document streams.
