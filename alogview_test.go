package main

import (
	"strings"
	"testing"
)

const testLog = `10-24 22:14:41.150 123 123 D TestTag1: first test message
10-24 22:14:41.150 123 123 D TestTag2: second test message
10-24 22:14:41.150 456 456 D TestTag1: third test message
10-24 22:14:41.150 456 456 D TestTag2: fourth test message`

func processFilter(line *logLine) bool {
	packages := map[string]bool{"foo": true}
	pids := map[int]bool{123: true}
	return filterByPackages(line, packages, pids)
}

func TestProcessFiltering(t *testing.T) {
	filters := []filter{processFilter}
	r := strings.NewReader(testLog)
	acc := []*logLine{}
	filterLogs(r, filters, func(line *logLine) { acc = append(acc, line) })
	if len(acc) != 2 {
		t.Error("Expected two log entries")
	}
	assertProcLine(t, acc[0], 123, "first test message")
	assertProcLine(t, acc[1], 123, "second test message")
}

func assertProcLine(t *testing.T, line *logLine, pid int, msg string) {
	if line.pid != pid {
		t.Errorf("invalid PID: expected: \"%d\" actual: \"%d\"", pid, line.pid)
	}
	if line.message != msg {
		t.Errorf("invalid message: expected: \"%s\" actual: \"%s\"", msg, line.message)
	}
}

func tagFilter(line *logLine) bool {
	tags := map[string]bool{"TestTag1": true}
	return filterByTags(line, tags)
}

func TestTagFiltering(t *testing.T) {
	filters := []filter{tagFilter}
	r := strings.NewReader(testLog)
	acc := []*logLine{}
	filterLogs(r, filters, func(line *logLine) { acc = append(acc, line) })
	if len(acc) != 2 {
		t.Error("expected two log entries")
	}
	assertTagLine(t, acc[0], "TestTag1", "first test message")
	assertTagLine(t, acc[1], "TestTag1", "third test message")
}

func assertTagLine(t *testing.T, line *logLine, tag string, msg string) {
	if line.tag != tag {
		t.Errorf("invalid tag: expected: \"%s\" actual: \"%s\"", tag, line.tag)
	}
	if line.message != msg {
		t.Errorf("invalid message: expected: \"%s\" actual: \"%s\"", msg, line.message)
	}
}

func TestProcessAndTagFiltering(t *testing.T) {
	filters := []filter{processFilter, tagFilter}
	r := strings.NewReader(testLog)
	acc := []*logLine{}
	filterLogs(r, filters, func(line *logLine) { acc = append(acc, line) })
	if len(acc) != 1 {
		t.Error("Expected a single log entries")
	}
	assertProcLine(t, acc[0], 123, "first test message")
	assertTagLine(t, acc[0], "TestTag1", "first test message")
}
