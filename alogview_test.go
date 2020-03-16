/*
 * Copyright (c) 2018-2020 Florian Limberger <flo@purplekraken.com>
 *
 * SPDX-License-Identifier: BSD-2-Clause
 */
package main

import (
	"strings"
	"testing"
)

const testLog = `10-24 22:14:41.150 123 123 D TestTag1: first test message
10-24 22:14:41.150 123 123 D TestTag2: second test message
10-24 22:14:41.150 456 456 D TestTag1: third test message
10-24 22:14:41.150 456 456 D TestTag2: fourth test message`

func runFilters(data string, filters []filter) []*logLine {
	r := strings.NewReader(data)
	acc := []*logLine{}
	rawlines := make(chan *logLine)
	filtered := startFilters(filters, rawlines)
	done := make(chan int)
	go func() {
		for {
			select {
			case line := <-filtered:
				acc = append(acc, line)
			case <-done:
				done <- 1
			}
		}
	}()
	parseLogs(r, rawlines)
	done <- 1
	<-done
	return acc
}

func createPackageFilter() *packageFilter {
	return &packageFilter{
		packages: map[string]bool{"foo": true},
		pids:     map[int]bool{123: true},
	}
}

func TestProcessFiltering(t *testing.T) {
	filters := []filter{createPackageFilter()}
	acc := runFilters(testLog, filters)
	// TODO: this is racy
	if len(acc) != 2 {
		t.Errorf("expected two log entries, got %d instead", len(acc))
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

func createTagFilter() *tagFilter {
	return &tagFilter{
		tags: map[string]bool{"TestTag1": true},
	}
}

func TestTagFiltering(t *testing.T) {
	filters := []filter{createTagFilter()}
	acc := runFilters(testLog, filters)
	if len(acc) != 2 {
		t.Errorf("expected two log entries, got %d instead", len(acc))
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
	filters := []filter{createPackageFilter(), createTagFilter()}
	acc := runFilters(testLog, filters)
	if len(acc) != 1 {
		t.Errorf("Expected a single log entry, got %d instead", len(acc))
	}
	assertProcLine(t, acc[0], 123, "first test message")
	assertTagLine(t, acc[0], "TestTag1", "first test message")
}
