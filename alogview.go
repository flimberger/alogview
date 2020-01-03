package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Custom type for multiple flags
type stringSetValue struct {
	values map[string]bool
}

type color int

const (
	black color = iota
	red
	green
	yellow
	blue
	magenta
	cyan
	white
)
const reset = "\033[0m"

type logLine struct {
	raw     string
	time    string
	pid     int
	tid     int
	level   string
	tag     string
	message string
}

// A filter implements the `filter()` method, which is expected to run as a goroutine.
type filter interface {
	filter(chan<- *logLine, <-chan *logLine)
}

type packageFilter struct {
	packages map[string]bool
	pids     map[int]bool
}

type tagFilter struct {
	tags map[string]bool
}

var (
	pslineMatcher    *regexp.Regexp
	loglineMatcher   *regexp.Regexp
	startprocMatcher *regexp.Regexp
	diedprocMatcher  *regexp.Regexp
	killprocMatcher  *regexp.Regexp

	// global store for additional ADB options
	adbcmd  = "adb"
	adbargs []string
)

func init() {
	// The line pattern is "${user:w} ${pid:d} ${ppid:d} ${vsz:d} ${rss:d} ${wchan:w} ${addr:w} ${s:w} ${name:w}"
	pslineMatcher = regexp.MustCompile(`\w+\s+(\d+)\s+\d+\s+\d+\s+\d+\s+\w+\s+\w+\s+[A-Z]\s+(.*)$`)

	// The line pattern is "${datetime} ${pid} ${tid} ${level} ${tag}: ${message}"
	// the datetime is in the format "MM-DD hh:mm:ss.sss"
	// the tag is optional, at least sometimes missing
	loglineMatcher = regexp.MustCompile(`(\d\d-\d\d \d\d:\d\d:\d\d.\d\d\d)\s+(\d+)\s+(\d+)\s+([DVIWEF])(.*?):\s+(.*)$`)

	// The start proc pattern is "Start Proc ${pid}:${package1}/${user} for ((activity|broadcast|service) ${package2}/${component})?"
	startprocMatcher = regexp.MustCompile(`Start proc (\d+):([A-Za-z0-9_.]+)/\w+`)
	// The died proc pattern is "Process ${package} (pid ${pid}) has died: ${reason}"
	diedprocMatcher = regexp.MustCompile(`Process ([A-Za-z0-9_.]+) \(pid (\d+)\) has died: .*$`)
	// The stop proc pattern is "Killing ${pid}:${package}/${user} (adj ${unknown}): ${reason}"
	killprocMatcher = regexp.MustCompile(`Killing (\d+):([A-Za-z0-9_.]+)/\w+ [^:]+: .*$`)
}

func main() {
	d := flag.Bool("d", false, "use USB device (error if multiple devices connected)")
	e := flag.Bool("e", false, "use TCP/IP device (error if multiple TCP/IP devices available)")
	h := flag.Bool("h", false, "show this help message")
	s := flag.String("s", "", "use device with given serial (overrides $ANDROID_SERIAL)")
	tags := newStringSetValue()
	flag.Var(tags, "t", "list of tags")
	flag.Parse()
	if *h {
		usage()
	}
	if *d && *e {
		fmt.Fprintln(os.Stderr, "invalid parameters: -e and -d must not be specified both")
		usage()
	}
	if *d {
		adbargs = append(adbargs, "-d")
	}
	if *e {
		adbargs = append(adbargs, "-e")
	}
	if len(*s) > 0 {
		adbargs = append(adbargs, "-s", *s)
	}
	adbenv, adbOverride := os.LookupEnv("ADB")
	if adbOverride {
		if len(adbenv) == 0 {
			fatal("ADB environment variable must not be set to empty string")
		}
		adbcmd = adbenv
	}
	_, suppresscolor := os.LookupEnv("NO_COLOR")
	filters := make([]filter, 0)
	if len(tags.values) > 0 {
		filters = append(filters, newTagFilter(tags.values))
	}
	if len(flag.Args()) > 0 {
		filters = append(filters, newPackageFilter(os.Args))
	}

	rawlines := make(chan *logLine)
	filtered := startFilters(filters, rawlines)

	go func() {
		if suppresscolor {
			for {
				line := <-filtered
				fmt.Printf("%s\n", line.raw)
			}
		} else {
			for {
				line := <-filtered
				fmt.Printf("%s%s%s\n", colorForLevel(line.level), line.raw, reset)
			}
		}
	}()

	r := startLogCollection()
	parseLogs(r, rawlines)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\t%s [-d|-e] [-s serial] [packagename]\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

// Start all filter functions as goroutines, with channels set up between them to send log lines down the chain.
func startFilters(filters []filter, rawlines chan *logLine) <-chan *logLine {
	linesout := rawlines

	for _, f := range filters {
		pipe := make(chan *logLine)
		go f.filter(pipe, linesout)
		linesout = pipe
	}

	return linesout
}

// Start an adb instance and return the reader end of the pipe.
func startLogCollection() io.Reader {
	r, w := io.Pipe()

	go runADB(w, "logcat")
	return r
}

// Read log lines from the reader, parse them into a logLine struct and send them to the linesout chan.
func parseLogs(r io.Reader, linesout chan<- *logLine) {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "--------- beginning of") {
			continue
		}
		msg, err := parseLine(line)

		if err != nil {
			warn(err)

			continue
		}
		linesout <- msg
	}
}

// Return the color escape code for the log level.
func colorForLevel(level string) string {
	s := ""

	switch level {
	case "V":
		s = termfg(white)
	case "D":
		s = termfg(cyan)
	case "I":
		s = termfg(green)
	case "W":
		s = termfg(yellow)
	case "E":
		s = termfg(red)
	case "F":
		s = termfg(magenta)
	}

	return s
}

func termfg(fg color) string {
	return fmt.Sprintf("\033[3%dm", fg)
}

func fatal(msg ...interface{}) {
	warn(msg...)
	os.Exit(1)
}

func warn(msg ...interface{}) {
	fmt.Fprintln(os.Stderr, msg...)
}

func runADB(out io.WriteCloser, args ...string) {
	args = append(adbargs, args...)
	cmd := exec.Command(adbcmd, args...)

	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fatal(err)
	}
	if err := out.Close(); err != nil {
		fatal(err)
	}
}

func parseLine(line string) (*logLine, error) {
	if parsed := loglineMatcher.FindStringSubmatch(line); parsed != nil {
		pid, e1 := strconv.Atoi(parsed[2])

		if e1 != nil {
			return nil, e1
		}

		tid, e2 := strconv.Atoi(parsed[3])

		if e2 != nil {
			return nil, e2
		}

		return &logLine{
			raw:     line,
			time:    parsed[1],
			pid:     pid,
			tid:     tid,
			level:   parsed[4],
			tag:     strings.TrimSpace(parsed[5]),
			message: parsed[6]}, nil
	}

	return nil, fmt.Errorf("failed to match log line \"%s\"", line)
}

// Create a new tagFilter from a list of tags.
func newTagFilter(tags map[string]bool) *tagFilter {
	return &tagFilter{
		tags: tags,
	}
}

func (tf *tagFilter) filter(out chan<- *logLine, in <-chan *logLine) {
	for {
		line := <-in
		if tf.tags[line.tag] {
			out <- line
		}
	}
}

// Create a packageFilter from a list of package names.
func newPackageFilter(pkgnames []string) *packageFilter {
	packages := make(map[string]bool)

	for _, pkg := range pkgnames {
		packages[pkg] = true
	}
	pids := getProcs(packages)
	if len(pids) == 0 {
		warn("no packages found matching the given package(s)")
	}
	return &packageFilter{
		packages: packages,
		pids:     pids,
	}
}

// Execute `adb shell ps` and parse the output to get a list of currently running processes; return the process IDs.
func getProcs(packages map[string]bool) map[int]bool {
	r, w := io.Pipe()

	go runADB(w, "shell", "ps")

	scanner := bufio.NewScanner(r)
	pids := make(map[int]bool)

	for scanner.Scan() {
		parsed := pslineMatcher.FindStringSubmatch(scanner.Text())

		if parsed != nil {
			pid := atoi(parsed[1])
			pkg := parsed[2]

			if packages[pkg] {
				pids[pid] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fatal(err)
	}

	return pids
}

func atoi(str string) int {
	i, err := strconv.Atoi(str)

	if err != nil {
		panic(err)
	}

	return i
}

func (pf *packageFilter) filter(out chan<- *logLine, in <-chan *logLine) {
	for {
		line := <-in
		if line.tag == "ActivityManager" && line.level == "I" {
			// start proc
			if parsedmsg := startprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
				pid := atoi(parsedmsg[1])
				pkg := parsedmsg[2]

				if pf.packages[pkg] {
					pf.pids[pid] = true

					out <- line
					continue
				}
			}
			// proc died
			if parsedmsg := diedprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
				pkg := parsedmsg[1]
				pid := atoi(parsedmsg[2])

				if pf.packages[pkg] || pf.pids[pid] {
					delete(pf.pids, pid)

					out <- line
					continue
				}
			}
			// proc killed
			if parsedmsg := killprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
				pid := atoi(parsedmsg[1])
				pkg := parsedmsg[2]

				if pf.packages[pkg] || pf.pids[pid] {
					delete(pf.pids, pid)

					out <- line
				}
			}
		} else if pf.pids[line.pid] {
			out <- line
		}
	}
}

func newStringSetValue() *stringSetValue {
	return &stringSetValue{
		values: make(map[string]bool),
	}
}

func (p *stringSetValue) String() string {
	accu := ""
	for k := range p.values {
		accu += k + ", "
	}
	return strings.TrimRight(accu, ", ")
}

func (p *stringSetValue) Set(value string) error {
	p.values[value] = true
	return nil
}

func (p *stringSetValue) Get() interface{} {
	return p.values
}
