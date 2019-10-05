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
	time    string
	pid     int
	tid     int
	level   string
	tag     string
	message string
}

// filter function type, it receives a parsed log line and returns bool if it will be logged
type filter func(*logLine) bool

var (
	pslineMatcher    *regexp.Regexp
	loglineMatcher   *regexp.Regexp
	startprocMatcher *regexp.Regexp
	diedprocMatcher  *regexp.Regexp
	killprocMatcher  *regexp.Regexp

	// global store for additional ADB options
	adbcmd  string = "adb"
	adbargs []string
)

func init() {
	// The line pattern is "${user} ${pid} ${ppid} ${vsz} $rss} ${wchan} ${addr} ${s} ${name}"
	pslineMatcher = regexp.MustCompile(`\w+\s+(\d+)\s+\d+\s+\d+\s+\d+\s+\d+\s+\w+\s+[A-Z]\s+(.*)$`)

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
	if len(flag.Args()) > 0 {
		packages := make(map[string]bool)

		for _, pkg := range os.Args {
			packages[pkg] = true
		}
		pids := getProcs(packages)
		filters = append(filters, func(line *logLine) bool { return filterByPackages(line, packages, pids) })
	}
	processLogs(filters, suppresscolor)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\t%s [-d|-e] [-s serial] [packagename]\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

func processLogs(filters []filter, suppresscolor bool) {
	r, w := io.Pipe()

	go runADB(w, "logcat")

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
		printline := true
		for _, filter := range filters {
			printline = filter(msg)
			if !printline {
				break
			}
		}
		if printline {
			if suppresscolor {
				fmt.Printf("%s\n", line)
			} else {
				fmt.Printf("%s%s%s\n", colorForLevel(msg.level), line, reset)
			}
		}
	}
}

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
			time:    parsed[1],
			pid:     pid,
			tid:     tid,
			level:   parsed[4],
			tag:     strings.TrimSpace(parsed[5]),
			message: parsed[6]}, nil
	}

	return nil, fmt.Errorf("failed to match log line \"%s\"", line)
}

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

func filterByPackages(line *logLine, packages map[string]bool, pids map[int]bool) bool {
	if line.tag == "ActivityManager" && line.level == "I" {
		// start proc
		if parsedmsg := startprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
			pid := atoi(parsedmsg[1])
			pkg := parsedmsg[2]

			if packages[pkg] {
				pids[pid] = true

				return true
			}
		}
		// proc died
		if parsedmsg := diedprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
			pkg := parsedmsg[1]
			pid := atoi(parsedmsg[2])

			if packages[pkg] || pids[pid] {
				delete(pids, pid)

				return true
			}
		}
		// proc killed
		if parsedmsg := killprocMatcher.FindStringSubmatch(line.message); parsedmsg != nil {
			pid := atoi(parsedmsg[1])
			pkg := parsedmsg[2]

			if packages[pkg] || pids[pid] {
				delete(pids, pid)

				return true
			}
		}
	}

	return pids[line.pid]
}
