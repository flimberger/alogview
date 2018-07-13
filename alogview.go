package main

import (
	"bufio"
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

var pslineMatcher *regexp.Regexp
var loglineMatcher *regexp.Regexp
var startprocMatcher *regexp.Regexp
var diedprocMatcher *regexp.Regexp
var killprocMatcher *regexp.Regexp

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
	if len(os.Args) == 1 {
		logAll()
	} else {
		logPackages(os.Args)
	}
}

func logAll() {
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
		fmt.Println(colorForLevel(msg.level), line, reset)
	}
}

func runADB(out io.WriteCloser, args ...string) {
	cmd := exec.Command("adb", args...)

	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fatal(err)
	}
	if err := out.Close(); err != nil {
		fatal(err)
	}
}

func logPackages(pkgs []string) {
	packages := make(map[string]bool)

	for _, pkg := range pkgs {
		packages[pkg] = true
	}
	pids := getProcs(packages)
	parseLogs(packages, pids)
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

func parseLogs(packages map[string]bool, pids map[int]bool) {
	r, w := io.Pipe()

	go runADB(w, "logcat")

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "--------- beginning of") {
			continue
		}
		parsedline, err := parseLine(line)

		if err != nil {
			warn(err)
			continue
		}
		if parsedline.tag == "ActivityManager" && parsedline.level == "I" {
			// start proc
			if parsedmsg := startprocMatcher.FindStringSubmatch(parsedline.message); parsedmsg != nil {
				pid := atoi(parsedmsg[1])
				pkg := parsedmsg[2]

				if packages[pkg] {
					pids[pid] = true
					fmt.Println(colorForLevel(parsedline.level), line, reset)
				}
			}
			// proc died
			if parsedmsg := diedprocMatcher.FindStringSubmatch(parsedline.message); parsedmsg != nil {
				pkg := parsedmsg[1]
				pid := atoi(parsedmsg[2])

				if packages[pkg] || pids[pid] {
					pids[pid] = false
					fmt.Println(colorForLevel(parsedline.level), line, reset)
				}
			}
			// proc killed
			if parsedmsg := killprocMatcher.FindStringSubmatch(parsedline.message); parsedmsg != nil {
				pid := atoi(parsedmsg[1])
				pkg := parsedmsg[2]

				if packages[pkg] || pids[pid] {
					pids[pid] = false
					fmt.Println(colorForLevel(parsedline.level), line, reset)
				}
			}
		}
		if pids[parsedline.pid] {
			fmt.Println(colorForLevel(parsedline.level), line, reset)
		}
	}
	if err := scanner.Err(); err != nil {
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
