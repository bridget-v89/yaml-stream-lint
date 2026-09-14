package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Finding is a single rule violation at a specific line.
type Finding struct {
	Line    int
	Rule    string
	Message string
}

func (f Finding) String() string {
	return fmt.Sprintf("%d: %s: %s", f.Line, f.Rule, f.Message)
}

// Options controls which checks run and their thresholds.
type Options struct {
	MaxLineLength int
}

func DefaultOptions() Options {
	return Options{MaxLineLength: 120}
}

var keyLineRe = regexp.MustCompile(`^( *)([^\s#][^:]*?):(\s|$)`)

// keyLevel tracks the set of keys already seen at one indentation depth.
// Only the levels on the current path down the document are kept, so
// memory use is bounded by nesting depth, not by document size.
type keyLevel struct {
	indent int
	seen   map[string]int // key -> line first seen
}

// Lint reads YAML line by line from r and reports findings through emit.
// It never buffers the whole input: at most one line, plus a small stack
// of per-indent key sets proportional to nesting depth, is held at a time.
func Lint(r io.Reader, opts Options, emit func(Finding)) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var stack []keyLevel
	lineNo := 0

	for {
		line, err := readLine(reader)
		if line == "" && err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		lineNo++

		checkTabs(line, lineNo, emit)
		checkTrailingWhitespace(line, lineNo, emit)
		checkLineLength(line, lineNo, opts.MaxLineLength, emit)
		checkDuplicateKey(line, lineNo, &stack, emit)
		checkFlowCollections(line, lineNo, emit)

		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// readLine returns the next line with its trailing newline stripped.
// It has no fixed size limit, unlike bufio.Scanner's default token size,
// so a single very long line can't make the linter fail outright.
func readLine(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		chunk, isPrefix, err := r.ReadLine()
		sb.Write(chunk)
		if !isPrefix {
			return sb.String(), err
		}
		if err != nil {
			return sb.String(), err
		}
	}
}

func checkTabs(line string, lineNo int, emit func(Finding)) {
	for _, r := range line {
		if r == ' ' {
			continue
		}
		if r == '\t' {
			emit(Finding{Line: lineNo, Rule: "no-tabs", Message: "tab used in indentation"})
		}
		break
	}
}

func checkTrailingWhitespace(line string, lineNo int, emit func(Finding)) {
	if len(line) == 0 {
		return
	}
	last := line[len(line)-1]
	if last == ' ' || last == '\t' {
		emit(Finding{Line: lineNo, Rule: "trailing-whitespace", Message: "trailing whitespace"})
	}
}

func checkLineLength(line string, lineNo, max int, emit func(Finding)) {
	if max <= 0 {
		return
	}
	if n := len([]rune(line)); n > max {
		emit(Finding{Line: lineNo, Rule: "line-length", Message: fmt.Sprintf("line is %d characters, exceeds %d", n, max)})
	}
}

func checkDuplicateKey(line string, lineNo int, stack *[]keyLevel, emit func(Finding)) {
	trimmed := strings.TrimLeft(line, " ")
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") ||
		strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// A line opening with a flow collection is a value, not a block
		// key, even though it may contain its own "word:" text that would
		// otherwise look like one. checkFlowCollections handles duplicate
		// keys inside it instead.
		return
	}
	m := keyLineRe.FindStringSubmatch(line)
	if m == nil {
		return
	}
	indent := len(m[1])
	key := strings.TrimSpace(m[2])

	// Drop levels deeper than or equal to the current indent that don't
	// match it exactly; a shallower indent closes out nested levels.
	for len(*stack) > 0 && (*stack)[len(*stack)-1].indent > indent {
		*stack = (*stack)[:len(*stack)-1]
	}

	if len(*stack) == 0 || (*stack)[len(*stack)-1].indent < indent {
		*stack = append(*stack, keyLevel{indent: indent, seen: map[string]int{}})
	}

	top := &(*stack)[len(*stack)-1]
	if firstLine, ok := top.seen[key]; ok {
		emit(Finding{Line: lineNo, Rule: "duplicate-key", Message: fmt.Sprintf("key %q already defined on line %d", key, firstLine)})
		return
	}
	top.seen[key] = lineNo
}

// flowFrame tracks one level of flow-collection nesting on a line. Only
// mapping frames carry a seen set; sequence frames just keep depth correct
// so a "," inside a nested [...] isn't mistaken for a mapping separator.
type flowFrame struct {
	isMap bool
	seen  map[string]int
}

var flowKeyRe = regexp.MustCompile(`^\s*('[^']*'|"[^"]*"|[^,:{}\[\]\s]+)\s*:`)

// checkFlowCollections finds duplicate keys inside flow-style mappings
// ({a: 1, b: 2}), including ones nested inside flow sequences or other
// mappings. It works on a single line at a time; a flow collection that
// wraps onto a following line isn't understood yet, so scanning simply
// stops at end of line.
func checkFlowCollections(line string, lineNo int, emit func(Finding)) {
	var stack []flowFrame

	tryKey := func(pos int) {
		if len(stack) == 0 || !stack[len(stack)-1].isMap {
			return
		}
		m := flowKeyRe.FindStringSubmatch(line[pos:])
		if m == nil {
			return
		}
		key := strings.Trim(m[1], `'"`)
		top := &stack[len(stack)-1]
		if _, ok := top.seen[key]; ok {
			emit(Finding{Line: lineNo, Rule: "duplicate-key", Message: fmt.Sprintf("key %q already defined earlier on this line", key)})
			return
		}
		top.seen[key] = pos
	}

	inSingle, inDouble := false, false
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			i++
		case inDouble:
			if c == '\\' && i+1 < len(line) {
				i += 2
				continue
			}
			if c == '"' {
				inDouble = false
			}
			i++
		case c == '\'':
			inSingle = true
			i++
		case c == '"':
			inDouble = true
			i++
		case c == '#':
			return
		case c == '{':
			stack = append(stack, flowFrame{isMap: true, seen: map[string]int{}})
			i++
			tryKey(i)
		case c == '[':
			stack = append(stack, flowFrame{})
			i++
		case c == '}' || c == ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			i++
		case c == ',':
			i++
			tryKey(i)
		default:
			i++
		}
	}
}
